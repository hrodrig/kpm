package live

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// podOnNode is podWithReq with a node, so it lands in the node aggregation.
func podOnNode(name, cpu string) corev1.Pod {
	p := podWithReq("app", name, cpu)
	p.Spec.NodeName = "n1"
	return p
}

// Terminal pods must not contribute requests. The scheduler releases a pod's
// reservation when it reaches Succeeded/Failed, and the Metrics API stops
// reporting it, so counting it inflates both the node total and the pod count —
// a finished Job would look like a permanently idle workload
// (kube-capacity tracks this as issue #99).
func TestTerminalPodsDoNotCountTowardNodeRequests(t *testing.T) {
	running := podOnNode("running", "500m")
	pending := podOnNode("pending", "500m")
	pending.Status.Phase = corev1.PodPending
	succeeded := podOnNode("job-done", "2")
	succeeded.Status.Phase = corev1.PodSucceeded
	failed := podOnNode("job-failed", "2")
	failed.Status.Phase = corev1.PodFailed

	rows := buildContainerRows([]corev1.Pod{running, pending, succeeded, failed}, nil)
	nodes := AggregateNodes([]corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}}, rows)

	if len(nodes) != 1 {
		t.Fatalf("node rows: got %d want 1", len(nodes))
	}
	// 500m running + 500m pending. Pending pods still hold their reservation;
	// the 2 CPU each from the two terminal pods must not appear.
	if nodes[0].CPUReqMcores != 1000 {
		t.Fatalf("node cpu request: got %v want 1000 (terminal pods must not count)", nodes[0].CPUReqMcores)
	}
	if nodes[0].Pods != 2 {
		t.Fatalf("node pod count: got %d want 2 (running + pending)", nodes[0].Pods)
	}
}

// A pod with no phase set yet (older objects, tests) must still be counted:
// the filter is an allowlist of terminal phases, not an allowlist of Running.
func TestPodsWithoutPhaseStillCount(t *testing.T) {
	rows := buildContainerRows([]corev1.Pod{podOnNode("unset", "250m")}, nil)
	nodes := AggregateNodes([]corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}}, rows)

	if len(nodes) != 1 || nodes[0].CPUReqMcores != 250 || nodes[0].Pods != 1 {
		t.Fatalf("phase-less pod dropped: %+v", nodes)
	}
}

// initContainer builds an init container requesting cpu.
func initContainer(name, cpu string) corev1.Container {
	return corev1.Container{
		Name: name,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)},
		},
	}
}

// A pod whose init container asks for more than its app containers sum to
// reserves the init amount of node capacity, not the sum. Counting only
// Spec.Containers understates how full the node is — the dangerous direction,
// because it reports headroom that does not exist (kube-capacity #53 and #74,
// where an open PR for exactly this has sat since 2026-05).
func TestInitContainerRequestsReachNodeTotals(t *testing.T) {
	p := podOnNode("migrate", "500m")
	p.Spec.InitContainers = []corev1.Container{initContainer("migrate", "2")}

	rows := buildContainerRows([]corev1.Pod{p}, nil)
	if len(rows) != 1 {
		t.Fatalf("rows: got %d want 1 (init containers are not container rows)", len(rows))
	}
	// The container row keeps its own 500m: rightsizing is per container, and an
	// init container consumes nothing at runtime.
	if rows[0].CPURequestMcores != 500 {
		t.Fatalf("container cpu request: got %v want 500", rows[0].CPURequestMcores)
	}
	if rows[0].PodCPURequestMcores != 2000 {
		t.Fatalf("pod effective cpu request: got %v want 2000", rows[0].PodCPURequestMcores)
	}

	nodes := AggregateNodes([]corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}}, rows)
	if nodes[0].CPUReqMcores != 2000 {
		t.Fatalf("node cpu request: got %v want 2000 (init container dominates)", nodes[0].CPUReqMcores)
	}
	if nodes[0].Pods != 1 {
		t.Fatalf("node pod count: got %d want 1", nodes[0].Pods)
	}
}

// When the app containers dominate, the effective request is their sum — the
// init container must not be added on top of it.
func TestContainerSumWinsWhenItExceedsInit(t *testing.T) {
	p := podOnNode("app", "1")
	p.Spec.Containers = append(p.Spec.Containers, corev1.Container{
		Name: "sidecar",
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
		},
	})
	p.Spec.InitContainers = []corev1.Container{initContainer("warm", "800m")}

	rows := buildContainerRows([]corev1.Pod{p}, nil)
	if rows[0].PodCPURequestMcores != 1500 {
		t.Fatalf("pod effective cpu request: got %v want 1500 (sum, not sum+init)", rows[0].PodCPURequestMcores)
	}

	nodes := AggregateNodes([]corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}}, rows)
	if nodes[0].CPUReqMcores != 1500 {
		t.Fatalf("node cpu request: got %v want 1500", nodes[0].CPUReqMcores)
	}
}

// Memory follows the same rule as CPU.
func TestInitContainerMemoryReachesNodeTotals(t *testing.T) {
	p := podOnNode("migrate", "100m")
	p.Spec.InitContainers = []corev1.Container{{
		Name: "migrate",
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
		},
	}}

	rows := buildContainerRows([]corev1.Pod{p}, nil)
	nodes := AggregateNodes([]corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}}, rows)
	want := int64(2 * 1024 * 1024 * 1024)
	if nodes[0].MemReqBytes != want {
		t.Fatalf("node mem request: got %d want %d", nodes[0].MemReqBytes, want)
	}
}
