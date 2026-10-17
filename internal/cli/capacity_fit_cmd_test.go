package cli

import (
	"bytes"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/hrodrig/kpm/internal/fit"
)

// The footer is not decoration: without it the table reads as a complete answer
// when several scheduler filters were never considered.
func TestWriteFitTable(t *testing.T) {
	out := &bytes.Buffer{}
	verdicts := []fit.Verdict{
		{Node: "free-0", Fits: true},
		{Node: "gpu-0", Reasons: []string{"taint dedicated=gpu:NoSchedule not tolerated"}},
	}
	if err := writeFitTable(out, fit.Pod{
		CPURequestMcores: 500,
		MemRequestBytes:  2 * 1024 * 1024 * 1024,
	}, verdicts); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{
		"cpu=500m", "memory=2.0Gi",
		"free-0", "yes",
		"gpu-0", "taint dedicated=gpu:NoSchedule not tolerated",
		"1 of 2 nodes fit",
		"pod (anti-)affinity",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestResolveFitPodFromFlags(t *testing.T) {
	pod, err := resolveFitPod(t.Context(), fake.NewSimpleClientset(), "", "default", "1", "4Gi")
	if err != nil {
		t.Fatal(err)
	}
	if pod.CPURequestMcores != 1000 {
		t.Fatalf("cpu: got %v want 1000", pod.CPURequestMcores)
	}
	if want := int64(4 * 1024 * 1024 * 1024); pod.MemRequestBytes != want {
		t.Fatalf("memory: got %d want %d", pod.MemRequestBytes, want)
	}
}

// A real workload's placement constraints are read from its pod template, and its
// request is the one the scheduler reserves -- max of the container sum and the
// init containers -- not the sum of Spec.Containers.
func TestResolveFitPodFromDeployment(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"pool": "general"},
			Tolerations: []corev1.Toleration{{
				Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule,
			}},
			Containers: []corev1.Container{{
				Name: "app",
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				}},
			}},
			InitContainers: []corev1.Container{{
				Name: "migrate",
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("2"),
				}},
			}},
		}}},
	}
	cs := fake.NewSimpleClientset(dep)

	pod, err := resolveFitPod(t.Context(), cs, "deployment/api", "prod", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if pod.CPURequestMcores != 2000 {
		t.Fatalf("cpu: got %v want 2000 (init container dominates)", pod.CPURequestMcores)
	}
	if pod.NodeSelector["pool"] != "general" {
		t.Fatalf("nodeSelector: %v", pod.NodeSelector)
	}
	if len(pod.Tolerations) != 1 || pod.Tolerations[0].Key != "dedicated" {
		t.Fatalf("tolerations: %v", pod.Tolerations)
	}
}

func TestResolveFitPodRejectsBadInput(t *testing.T) {
	cs := fake.NewSimpleClientset()

	if _, err := resolveFitPod(t.Context(), cs, "api", "prod", "", ""); err == nil {
		t.Fatal("expected kind/name error")
	}
	if _, err := resolveFitPod(t.Context(), cs, "secret/x", "prod", "", ""); err == nil {
		t.Fatal("expected unsupported kind error")
	}
	if _, err := resolveFitPod(t.Context(), cs, "deployment/missing", "prod", "", ""); err == nil {
		t.Fatal("expected not-found error")
	}
}

// fitNodes reads what is reserved from pod specs only, so a fit check never needs
// the Metrics API. Terminal pods must not occupy capacity, and the node's pool
// identity (labels, taints, cordon) has to survive the trip.
func TestFitNodesMapsNodesAndRequests(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"pool": "general"}},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{{
			Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule,
		}}},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		}},
	}
	running := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "app"},
		Spec: corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{
			Name: "c",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("500m"),
			}},
		}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	done := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "app"},
		Spec: corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{
			Name: "c",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("2"),
			}},
		}}},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}

	nodes, err := fitNodes(t.Context(), fake.NewSimpleClientset(node, running, done))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes: got %d want 1", len(nodes))
	}
	n := nodes[0]
	if n.CPUAllocMcores != 4000 {
		t.Fatalf("alloc: got %v want 4000", n.CPUAllocMcores)
	}
	if n.CPUReqMcores != 500 {
		t.Fatalf("requests: got %v want 500 (the Succeeded pod must not count)", n.CPUReqMcores)
	}
	if n.Labels["pool"] != "general" {
		t.Fatalf("labels: %v", n.Labels)
	}
	if len(n.Taints) != 1 || n.Taints[0].Key != "dedicated" {
		t.Fatalf("taints: %v", n.Taints)
	}
}
