package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hrodrig/kpm/internal/live"
)

// The node table has to make two things visible: which pool a node belongs to,
// and that a tainted node still contributes its capacity to the pool total.
func TestWriteNodeTableShowsPools(t *testing.T) {
	nodes := []live.NodeCapacity{
		{Name: "gen-0", CPUAllocMcores: 4000, CPUReqMcores: 2000, CPUUtilMcores: 500, Pods: 5},
		{Name: "gpu-0", Taints: []string{"dedicated=gpu:NoSchedule"}, CPUAllocMcores: 8000, Pods: 2},
		{Name: "gpu-1", Taints: []string{"dedicated=gpu:NoSchedule"}, CPUAllocMcores: 8000, Unschedulable: true},
	}

	out := &bytes.Buffer{}
	if err := writeNodeTable(out, nodes); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	for _, want := range []string{
		"NODE", "POOL", "ALLOC_CPU", "REQ_CPU", "UTIL_CPU",
		"gen-0", live.GeneralPool,
		"gpu-0", "dedicated=gpu:NoSchedule", "(cordoned)",
		"CORDONED", "ALLOC_MEM",
		// the pool total keeps both nodes' capacity
		"16000m",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWriteNodeTableEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	if err := writeNodeTable(out, nil); err != nil {
		t.Fatal(err)
	}
	// Header only: no pool summary section (its header is the only one with
	// CORDONED; the per-node header names POOL as a column).
	if strings.Contains(out.String(), "CORDONED") {
		t.Fatalf("unexpected pool summary: %q", out.String())
	}
}
