package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/store"
)

func TestPrepareRowsAndCycle(t *testing.T) {
	snap := live.Snapshot{Containers: []live.ContainerRow{
		{Namespace: "a", Pod: "hot", Container: "c", HasUtil: true,
			CPUUtilMcores: 90, CPURequestMcores: 100, MemUtilBytes: 10, MemRequestBytes: 100},
		{Namespace: "a", Pod: "cool", Container: "c", HasUtil: true,
			CPUUtilMcores: 10, CPURequestMcores: 100, MemUtilBytes: 10, MemRequestBytes: 100},
	}, CapturedAt: time.Now().UTC()}
	idx := IndexRecommendations([]store.Recommendation{{
		Grain: "pod", GrainKey: "a/hot/c", Namespace: "a", Pod: "hot", Container: "c",
		CPUSugMcores: 70, MemSugBytes: 50,
	}})
	rows := PrepareRows(snap, idx, live.SortCPURatio, MetricCPU, BaselineRequest)
	if len(rows) != 2 || rows[0].Pod != "hot" {
		t.Fatalf("sort hottest first: %+v", rows)
	}
	if !rows[0].Delta.HasSnapshot || rows[1].Delta.HasSnapshot {
		t.Fatalf("delta attach: %+v", rows)
	}
	if CycleSort(live.SortCPURatio) != live.SortMemRatio {
		t.Fatal("cycle")
	}
	if ParseBaseline("limit") != BaselineLimit {
		t.Fatal("baseline")
	}
	h := HeaderLine("v0.2.0", 2, MetricCPU, BaselineRequest, live.SortCPURatio, true, "")
	if !strings.Contains(h, "kpm v0.2.0") || !strings.Contains(h, "delta=on") {
		t.Fatalf("header: %s", h)
	}
	if !strings.Contains(HeaderLine("", 0, MetricCPU, BaselineRequest, live.SortName, false, "x"), "kpm dev") {
		t.Fatal("empty version → dev")
	}
	if FormatIdentity("ns", "very-long-pod-name-here", 12) == "ns/very-long-pod-name-here" {
		t.Fatal("expected truncate")
	}
}

func TestPrepareRowsLimitBaseline(t *testing.T) {
	snap := live.Snapshot{Containers: []live.ContainerRow{{
		Namespace: "a", Pod: "p", Container: "c", HasUtil: true,
		CPUUtilMcores: 50, CPULimitMcores: 100, MemUtilBytes: 25, MemLimitBytes: 100,
	}}}
	rows := PrepareRows(snap, RecIndex{}, live.SortName, MetricCPU, BaselineLimit)
	if len(rows) != 1 || !rows[0].HasRatio || rows[0].Ratio != 0.5 {
		t.Fatalf("limit ratio: %+v", rows)
	}
	rows = PrepareRows(snap, RecIndex{}, live.SortName, MetricMem, BaselineLimit)
	if !rows[0].HasRatio || rows[0].Ratio != 0.25 {
		t.Fatalf("mem limit: %+v", rows[0])
	}
	if MetricLabel(MetricMem) != "mem" || BaselineLabel(BaselineLimit) != "limit" {
		t.Fatal("labels")
	}
	if CycleSort(live.SortName) != live.SortCPURatio {
		t.Fatal("cycle wrap")
	}
}
