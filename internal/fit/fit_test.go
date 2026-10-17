package fit

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

const gi = int64(1024 * 1024 * 1024)

func node(name string, cpuM float64) Node {
	return Node{Name: name, CPUAllocMcores: cpuM, MemAllocBytes: 4 * gi}
}

func gpuTaint() corev1.Taint {
	return corev1.Taint{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule}
}

// A dedicated pool is available to the workload it exists for, and to nobody
// else. That asymmetry is the whole point of a taint, and it is why capacity
// totals must never be filtered by it.
func TestTaintBlocksUnlessTolerated(t *testing.T) {
	n := node("gpu-0", 4000)
	n.Taints = []corev1.Taint{gpuTaint()}

	pod := Pod{CPURequestMcores: 100}
	vs := Evaluate(pod, []Node{n})
	if vs[0].Fits {
		t.Fatal("an untolerating pod must not fit a tainted node")
	}
	if !strings.Contains(vs[0].Reasons[0], "dedicated=gpu:NoSchedule") {
		t.Fatalf("reason should name the taint: %v", vs[0].Reasons)
	}

	pod.Tolerations = []corev1.Toleration{{
		Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "gpu", Effect: corev1.TaintEffectNoSchedule,
	}}
	if vs = Evaluate(pod, []Node{n}); !vs[0].Fits {
		t.Fatalf("a tolerating pod should fit: %v", vs[0].Reasons)
	}
}

// PreferNoSchedule is a preference. Treating it as a filter would hide capacity
// that a pod can actually use.
func TestPreferNoScheduleDoesNotBlock(t *testing.T) {
	n := node("soft-0", 4000)
	n.Taints = []corev1.Taint{{Key: "x", Value: "y", Effect: corev1.TaintEffectPreferNoSchedule}}

	if vs := Evaluate(Pod{CPURequestMcores: 100}, []Node{n}); !vs[0].Fits {
		t.Fatalf("PreferNoSchedule must not block: %v", vs[0].Reasons)
	}
}

// NoExecute blocks like NoSchedule, and additionally evicts running pods. Both
// must be treated as filters.
func TestNoExecuteAlsoBlocks(t *testing.T) {
	n := node("evict-0", 4000)
	n.Taints = []corev1.Taint{{Key: "maintenance", Effect: corev1.TaintEffectNoExecute}}

	vs := Evaluate(Pod{CPURequestMcores: 100}, []Node{n})
	if vs[0].Fits {
		t.Fatal("NoExecute must block an untolerating pod")
	}
	if !strings.Contains(vs[0].Reasons[0], "maintenance:NoExecute") {
		t.Fatalf("reason: %v", vs[0].Reasons)
	}
}

func TestTolerationMatching(t *testing.T) {
	taint := gpuTaint()
	cases := []struct {
		name string
		tol  corev1.Toleration
		want bool
	}{
		{"equal", corev1.Toleration{Key: "dedicated", Value: "gpu", Effect: taint.Effect}, true},
		{"value differs", corev1.Toleration{Key: "dedicated", Value: "cpu", Effect: taint.Effect}, false},
		{"key differs", corev1.Toleration{Key: "other", Value: "gpu", Effect: taint.Effect}, false},
		{"empty effect matches any", corev1.Toleration{Key: "dedicated", Value: "gpu"}, true},
		{"effect differs", corev1.Toleration{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoExecute}, false},
		{"exists on key", corev1.Toleration{Key: "dedicated", Operator: corev1.TolerationOpExists, Effect: taint.Effect}, true},
		{"exists with empty key matches all", corev1.Toleration{Operator: corev1.TolerationOpExists}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tolerated([]corev1.Toleration{tc.tol}, taint); got != tc.want {
				t.Fatalf("tolerated = %v want %v", got, tc.want)
			}
		})
	}
}

func TestInsufficientCapacitySpellsOutTheGap(t *testing.T) {
	n := node("small", 500)
	n.CPUReqMcores = 400

	vs := Evaluate(Pod{CPURequestMcores: 200, MemRequestBytes: 8 * gi}, []Node{n})
	if vs[0].Fits {
		t.Fatal("expected the node not to fit")
	}
	joined := strings.Join(vs[0].Reasons, " | ")
	if !strings.Contains(joined, "insufficient cpu: needs 200m, free 100m") {
		t.Fatalf("cpu reason: %q", joined)
	}
	if !strings.Contains(joined, "insufficient memory: needs 8.0Gi, free 4.0Gi") {
		t.Fatalf("memory reason: %q", joined)
	}
}

func TestCordonedNodeDoesNotFit(t *testing.T) {
	n := node("drained", 4000)
	n.Unschedulable = true

	vs := Evaluate(Pod{CPURequestMcores: 100}, []Node{n})
	if vs[0].Fits || !strings.Contains(strings.Join(vs[0].Reasons, ","), "cordoned") {
		t.Fatalf("cordoned node: %+v", vs[0])
	}
}

func TestNodeSelectorAndAffinity(t *testing.T) {
	n := node("n1", 4000)
	n.Labels = map[string]string{"pool": "gpu", "disktype": "ssd", "cores": "16"}

	pod := Pod{
		CPURequestMcores: 100,
		NodeSelector:     map[string]string{"pool": "gpu"},
		NodeAffinity: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{Key: "disktype", Operator: corev1.NodeSelectorOpIn, Values: []string{"ssd", "nvme"}},
				{Key: "cores", Operator: corev1.NodeSelectorOpGt, Values: []string{"8"}},
				{Key: "gpu", Operator: corev1.NodeSelectorOpDoesNotExist},
			},
		}}},
	}
	if vs := Evaluate(pod, []Node{n}); !vs[0].Fits {
		t.Fatalf("expected a fit: %v", vs[0].Reasons)
	}

	pod.NodeSelector = map[string]string{"pool": "cpu"}
	vs := Evaluate(pod, []Node{n})
	if vs[0].Fits || !strings.Contains(vs[0].Reasons[0], "nodeSelector pool=cpu") {
		t.Fatalf("selector mismatch: %+v", vs[0])
	}
}

// Terms are ORed, expressions inside a term are ANDed, as the scheduler does.
func TestAffinityTermsAreOred(t *testing.T) {
	labels := map[string]string{"zone": "b"}

	sel := &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{
		{MatchExpressions: []corev1.NodeSelectorRequirement{
			{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"}},
		}},
		{MatchExpressions: []corev1.NodeSelectorRequirement{
			{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"b"}},
		}},
	}}
	if !affinityMatches(sel, labels) {
		t.Fatal("second term should match")
	}

	sel.NodeSelectorTerms[1].MatchExpressions = append(
		sel.NodeSelectorTerms[1].MatchExpressions,
		corev1.NodeSelectorRequirement{Key: "rack", Operator: corev1.NodeSelectorOpIn, Values: []string{"r1"}},
	)
	if affinityMatches(sel, labels) {
		t.Fatal("expressions inside one term must all match")
	}
}

// A non-integer label never satisfies Gt/Lt rather than being coerced.
func TestGtLtRejectsNonIntegers(t *testing.T) {
	e := corev1.NodeSelectorRequirement{Key: "cores", Operator: corev1.NodeSelectorOpGt, Values: []string{"8"}}
	if matchExpr(e, map[string]string{"cores": "many"}) {
		t.Fatal("non-integer label must not match Gt")
	}
	if !matchExpr(e, map[string]string{"cores": "9"}) {
		t.Fatal("9 > 8 should match")
	}
	if matchExpr(e, map[string]string{}) {
		t.Fatal("missing label must not match Gt")
	}
}

func TestFittingListsOnlySchedulableNodes(t *testing.T) {
	free := node("free-0", 4000)
	busy := node("busy-0", 100)
	busy.CPUReqMcores = 100

	got := Fitting(Evaluate(Pod{CPURequestMcores: 200}, []Node{free, busy}))
	if len(got) != 1 || got[0] != "free-0" {
		t.Fatalf("fitting: %v", got)
	}
}
