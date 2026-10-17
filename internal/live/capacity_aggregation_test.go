package live

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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
