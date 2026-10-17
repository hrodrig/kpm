package live

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/hrodrig/kpm/internal/config"
)

// PodLister lists pods (narrow interface for tests).
type PodLister interface {
	List(ctx context.Context, namespace string, opts metav1.ListOptions) (*corev1.PodList, error)
}

// MetricsLister lists pod metrics.
type MetricsLister interface {
	List(ctx context.Context, namespace string, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error)
}

type k8sPodLister struct {
	cs kubernetes.Interface
}

func (l k8sPodLister) List(ctx context.Context, namespace string, opts metav1.ListOptions) (*corev1.PodList, error) {
	return l.cs.CoreV1().Pods(namespace).List(ctx, opts)
}

type k8sMetricsLister struct {
	cs metricsclientset.Interface
}

func (l k8sMetricsLister) List(ctx context.Context, namespace string, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error) {
	return l.cs.MetricsV1beta1().PodMetricses(namespace).List(ctx, opts)
}

// Options configures one live snapshot.
type Options struct {
	Pods    PodLister
	Metrics MetricsLister
	Scope   config.Scope
	// PodAllowlist is optional namespace/name entries (tui.pods). Empty = all in scope.
	PodAllowlist []string
	// ExtraPodLabels is ANDed with scope.pod_labels (tui.pod_labels).
	ExtraPodLabels string
	Now            func() time.Time
}

// Snapshot is the result of one live capture (container grain).
type Snapshot struct {
	Containers []ContainerRow
	CapturedAt time.Time
}

// Capture lists pods + metrics under scope and builds container rows.
// Pods without metrics are kept with HasUtil=false (unlike sample→SQLite).
func Capture(ctx context.Context, opt Options) (Snapshot, error) {
	if opt.Pods == nil || opt.Metrics == nil {
		return Snapshot{}, fmt.Errorf("live: pods and metrics listers required")
	}
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}

	namespaces := opt.Scope.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	listOpts := metav1.ListOptions{}
	if sel := joinLabelSelectors(opt.Scope.PodLabels, opt.ExtraPodLabels); sel != "" {
		listOpts.LabelSelector = sel
	}

	var pods []corev1.Pod
	var metrics []metricsv1beta1.PodMetrics
	for _, ns := range namespaces {
		pl, err := opt.Pods.List(ctx, ns, listOpts)
		if err != nil {
			return Snapshot{}, fmt.Errorf("list pods: %w", err)
		}
		pods = append(pods, pl.Items...)

		ml, err := opt.Metrics.List(ctx, ns, listOpts)
		if err != nil {
			return Snapshot{}, fmt.Errorf("list pod metrics: %w", err)
		}
		metrics = append(metrics, ml.Items...)
	}

	pods = filterAllowlist(pods, opt.PodAllowlist)
	rows := buildContainerRows(pods, metrics)
	return Snapshot{Containers: rows, CapturedAt: now().UTC()}, nil
}

// CaptureClients runs Capture with typed clientsets.
func CaptureClients(ctx context.Context, pods kubernetes.Interface, metrics metricsclientset.Interface, opt Options) (Snapshot, error) {
	opt.Pods = k8sPodLister{cs: pods}
	opt.Metrics = k8sMetricsLister{cs: metrics}
	return Capture(ctx, opt)
}

func joinLabelSelectors(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

func filterAllowlist(pods []corev1.Pod, allow []string) []corev1.Pod {
	if len(allow) == 0 {
		return pods
	}
	want := make(map[string]struct{}, len(allow))
	for _, e := range allow {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		want[e] = struct{}{}
	}
	if len(want) == 0 {
		return pods
	}
	var out []corev1.Pod
	for i := range pods {
		key := pods[i].Namespace + "/" + pods[i].Name
		if _, ok := want[key]; ok {
			out = append(out, pods[i])
		}
	}
	return out
}

func buildContainerRows(pods []corev1.Pod, metrics []metricsv1beta1.PodMetrics) []ContainerRow {
	byKey := make(map[string]metricsv1beta1.PodMetrics, len(metrics))
	for _, m := range metrics {
		byKey[m.Namespace+"/"+m.Name] = m
	}

	var out []ContainerRow
	for i := range pods {
		p := &pods[i]
		// Succeeded/Failed pods no longer hold their requests: the scheduler
		// released them when the pod reached a terminal phase, and the Metrics API
		// stops reporting them. Counting them inflates node requests and the pod
		// count, and makes finished Jobs look like permanently idle workloads.
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			continue
		}
		ownerKind, ownerName := ownerRef(p)
		reqCPU, reqMem, limCPU, limMem := requestsLimits(p)
		m, hasMetrics := byKey[p.Namespace+"/"+p.Name]
		utilByContainer := map[string]metricsv1beta1.ContainerMetrics{}
		if hasMetrics {
			for _, c := range m.Containers {
				utilByContainer[c.Name] = c
			}
		}
		for _, c := range p.Spec.Containers {
			row := ContainerRow{
				Namespace:        p.Namespace,
				Pod:              p.Name,
				Container:        c.Name,
				OwnerKind:        ownerKind,
				OwnerName:        ownerName,
				Node:             p.Spec.NodeName,
				CPURequestMcores: milliCPU(reqCPU[c.Name]),
				MemRequestBytes:  bytesQty(reqMem[c.Name]),
				CPULimitMcores:   milliCPU(limCPU[c.Name]),
				MemLimitBytes:    bytesQty(limMem[c.Name]),
			}
			if um, ok := utilByContainer[c.Name]; ok {
				row.HasUtil = true
				row.CPUUtilMcores = milliCPU(um.Usage.Cpu())
				row.MemUtilBytes = bytesQty(um.Usage.Memory())
			}
			out = append(out, row)
		}
	}
	return out
}

func ownerRef(p *corev1.Pod) (kind, name string) {
	if len(p.OwnerReferences) == 0 {
		return "", ""
	}
	o := p.OwnerReferences[0]
	return o.Kind, o.Name
}

func requestsLimits(p *corev1.Pod) (reqCPU, reqMem, limCPU, limMem map[string]*resource.Quantity) {
	reqCPU = map[string]*resource.Quantity{}
	reqMem = map[string]*resource.Quantity{}
	limCPU = map[string]*resource.Quantity{}
	limMem = map[string]*resource.Quantity{}
	for _, c := range p.Spec.Containers {
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			cp := q
			reqCPU[c.Name] = &cp
		}
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			cp := q
			reqMem[c.Name] = &cp
		}
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			cp := q
			limCPU[c.Name] = &cp
		}
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			cp := q
			limMem[c.Name] = &cp
		}
	}
	return reqCPU, reqMem, limCPU, limMem
}

func milliCPU(q *resource.Quantity) float64 {
	if q == nil {
		return 0
	}
	return float64(q.MilliValue())
}

func bytesQty(q *resource.Quantity) int64 {
	if q == nil {
		return 0
	}
	return q.Value()
}
