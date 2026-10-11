package recommend

import (
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/store"
)

func TestSnapshotsFromResult(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	res := Result{
		WindowStart: now.AddDate(0, 0, -7),
		WindowEnd:   now,
		Percentile:  95,
		TargetUtil:  0.7,
		Advice: []Advice{{
			Grain: GrainPod, Key: "ns/p/c", Namespace: "ns", Pod: "p", Container: "c",
			Samples: 30, CPUPercentile: 40, MemPercentile: 16 << 20,
			CPUSuggested: 60, MemSuggested: 32 << 20, CPURequest: 100, MemRequest: 64 << 20,
		}},
	}
	rows := SnapshotsFromResult(res, "cid", "demo", now)
	if len(rows) != 1 {
		t.Fatalf("len: %d", len(rows))
	}
	r := rows[0]
	if r.ClientID != "cid" || r.ClusterName != "demo" || r.GrainKey != "ns/p/c" {
		t.Fatalf("identity/key: %+v", r)
	}
	if r.WindowDays != 7 {
		t.Fatalf("window_days: %d", r.WindowDays)
	}
	if r.CPUSugMcores != 60 || r.MemSugBytes != 32<<20 {
		t.Fatalf("sug: %+v", r)
	}
}

func TestDeltaFromLive(t *testing.T) {
	if d := DeltaFromLive(100, 64<<20, nil); d.HasSnapshot {
		t.Fatal("nil snap")
	}
	snap := &store.Recommendation{
		CPUSugMcores: 80, MemSugBytes: 32 << 20,
		RecordedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	d := DeltaFromLive(100, 64<<20, snap)
	if !d.HasSnapshot || d.CPUReqDelta != 20 || d.MemReqDelta != 32<<20 {
		t.Fatalf("delta: %+v", d)
	}
}
