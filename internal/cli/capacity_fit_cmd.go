package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/fit"
	"github.com/hrodrig/kpm/internal/live"
)

func newCapacityFitCmd() *cobra.Command {
	var configPath, kubeconfig, namespace, workload, cpuStr, memStr string

	cmd := &cobra.Command{
		Use:   "fit",
		Short: "Check which nodes could run a workload",
		Long: `Answer "can this workload be scheduled here", which is a different question
from "how much capacity is free".

Evaluates the filters that can be decided from the pod and the nodes alone:
cordon, taints and tolerations, nodeSelector, required node affinity, and the
CPU/memory a node has left after what is already reserved.

It does NOT evaluate ` + fit.Unmodelled + `. Those need the rest of the cluster
state, and the output says so instead of implying they were considered.

Use --for to read the requests, tolerations and selectors from a real workload, or
--cpu/--memory for a hypothetical pod.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(workload) == "" && strings.TrimSpace(cpuStr) == "" {
				return errors.New("one of --for (e.g. --for deployment/api) or --cpu is required")
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			restCfg, err := cluster.LoadRESTConfig(kubeconfig, cfg.Kube.Context)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}
			cs, err := cluster.NewClientset(restCfg)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}

			pod, err := resolveFitPod(cmd.Context(), cs, workload, namespace, cpuStr, memStr)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}
			nodes, err := fitNodes(cmd.Context(), cs)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}
			fitting, err := writeFitTable(cmd.OutOrStdout(), pod, fit.Evaluate(pod, nodes))
			if err != nil {
				return err
			}
			return fitResult(fitting)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: discovery / in-cluster)")
	cmd.Flags().StringVar(&workload, "for", "", "workload as kind/name: deployment, statefulset or daemonset")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "namespace of the workload")
	cmd.Flags().StringVar(&cpuStr, "cpu", "", "cpu request for a hypothetical pod (e.g. 500m or 2)")
	cmd.Flags().StringVar(&memStr, "memory", "", "memory request for a hypothetical pod (e.g. 4Gi)")
	return cmd
}

// resolveFitPod reads the placement-relevant parts of a real workload, or builds
// a bare pod from the --cpu/--memory flags.
func resolveFitPod(ctx context.Context, cs kubernetes.Interface, workload, namespace, cpuStr, memStr string) (fit.Pod, error) {
	if strings.TrimSpace(workload) == "" {
		return podFromFlags(cpuStr, memStr)
	}
	kind, name, err := splitWorkload(workload)
	if err != nil {
		return fit.Pod{}, err
	}
	spec, err := workloadPodSpec(ctx, cs, kind, name, namespace)
	if err != nil {
		return fit.Pod{}, err
	}
	return podFromSpec(&spec), nil
}

// splitWorkload parses kind/name, accepting the usual short aliases.
func splitWorkload(workload string) (kind, name string, err error) {
	parts := strings.SplitN(workload, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("--for must be kind/name, e.g. --for deployment/api")
	}
	return strings.ToLower(parts[0]), parts[1], nil
}

func podFromFlags(cpuStr, memStr string) (fit.Pod, error) {
	cpuQ, err := resource.ParseQuantity(cpuStr)
	if err != nil {
		return fit.Pod{}, fmt.Errorf("--cpu: %w", err)
	}
	var memB int64
	if strings.TrimSpace(memStr) != "" {
		memQ, err := resource.ParseQuantity(memStr)
		if err != nil {
			return fit.Pod{}, fmt.Errorf("--memory: %w", err)
		}
		memB = memQ.Value()
	}
	return fit.Pod{CPURequestMcores: float64(cpuQ.MilliValue()), MemRequestBytes: memB}, nil
}

// workloadPodSpec reads the pod template of the named workload.
func workloadPodSpec(ctx context.Context, cs kubernetes.Interface, kind, name, namespace string) (corev1.PodSpec, error) {
	switch kind {
	case "deployment", "deploy":
		d, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return corev1.PodSpec{}, workloadErr(kind, name, namespace, err)
		}
		return d.Spec.Template.Spec, nil
	case "statefulset", "sts":
		s, err := cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return corev1.PodSpec{}, workloadErr(kind, name, namespace, err)
		}
		return s.Spec.Template.Spec, nil
	case "daemonset", "ds":
		d, err := cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return corev1.PodSpec{}, workloadErr(kind, name, namespace, err)
		}
		return d.Spec.Template.Spec, nil
	}
	return corev1.PodSpec{}, fmt.Errorf("unsupported workload kind %q (deployment, statefulset, daemonset)", kind)
}

func workloadErr(kind, name, namespace string, err error) error {
	return fmt.Errorf("%s/%s in namespace %s: %w", kind, name, namespace, err)
}

// podFromSpec keeps the parts of a pod template that decide placement.
func podFromSpec(spec *corev1.PodSpec) fit.Pod {
	cpuM, memB := live.EffectivePodRequest(spec)
	pod := fit.Pod{
		CPURequestMcores: cpuM,
		MemRequestBytes:  memB,
		Tolerations:      spec.Tolerations,
		NodeSelector:     spec.NodeSelector,
	}
	if spec.Affinity != nil && spec.Affinity.NodeAffinity != nil {
		pod.NodeAffinity = spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	}
	return pod
}

// fitNodes lists nodes and pods and turns them into fit candidates. Requests come
// from the pod specs alone: a fit check must not depend on the Metrics API being
// readable, and utilisation has no bearing on whether something is placed.
func fitNodes(ctx context.Context, cs kubernetes.Interface) ([]fit.Node, error) {
	nodeList, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	podList, err := cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	caps := live.AggregateNodes(nodeList.Items, live.ContainerRowsFromPods(podList.Items))

	byName := make(map[string]*corev1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		byName[nodeList.Items[i].Name] = &nodeList.Items[i]
	}
	out := make([]fit.Node, 0, len(caps))
	for _, c := range caps {
		n := byName[c.Name]
		row := fit.Node{
			Name:           c.Name,
			CPUAllocMcores: c.CPUAllocMcores,
			MemAllocBytes:  c.MemAllocBytes,
			CPUReqMcores:   c.CPUReqMcores,
			MemReqBytes:    c.MemReqBytes,
			Unschedulable:  c.Unschedulable,
		}
		if n != nil {
			row.Labels = n.Labels
			row.Taints = n.Spec.Taints
		}
		out = append(out, row)
	}
	return out, nil
}

// fitResult turns the number of fitting nodes into the command's outcome.
//
// Zero fits is an answer, not a failure to answer: exit 4 tells a script that the
// cluster was asked and said no, which is different from being unable to ask (2)
// or having no data (3).
func fitResult(fitting int) error {
	if fitting > 0 {
		return nil
	}
	return exitcode.New(exitcode.NoFit, errors.New("no node can run this workload"))
}

func writeFitTable(out io.Writer, pod fit.Pod, verdicts []fit.Verdict) (int, error) {
	_, _ = fmt.Fprintf(out, "request: cpu=%s memory=%s\n\n",
		fit.FormatMilliCPU(pod.CPURequestMcores), fit.FormatBytes(pod.MemRequestBytes))
	_, _ = fmt.Fprintf(out, "%-30s %-5s %s\n", "NODE", "FITS", "REASON")

	fitting := 0
	for _, v := range verdicts {
		status, reason := "no", strings.Join(v.Reasons, "; ")
		if v.Fits {
			status, reason = "yes", ""
			fitting++
		}
		_, _ = fmt.Fprintf(out, "%-30s %-5s %s\n", v.Node, status, reason)
	}

	_, _ = fmt.Fprintf(out, "\n%d of %d nodes fit\n", fitting, len(verdicts))
	_, _ = fmt.Fprintf(out, "note: not evaluated: %s\n", fit.Unmodelled)
	return fitting, nil
}
