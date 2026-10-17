package recommend

import (
	"math"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func splitFixtures(now time.Time, cpuM, memBytes float64, n int) []store.Record {
	out := make([]store.Record, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, store.Record{
			SampledAt:        now.Add(-time.Duration(i) * time.Hour),
			Namespace:        "ns",
			Pod:              "web-abc",
			Container:        "app",
			OwnerKind:        "Deployment",
			OwnerName:        "web",
			CPUUtilMcores:    cpuM,
			MemUtilBytes:     int64(memBytes),
			CPURequestMcores: 1000,
			MemRequestBytes:  int64(memBytes),
		})
	}
	return out
}

func splitAdvice(t *testing.T, res Result) Advice {
	t.Helper()
	for _, a := range res.Advice {
		if a.Grain == GrainWorkload && a.Key == "Deployment/web/app" {
			return a
		}
	}
	t.Fatalf("no workload advice in %+v", res.Advice)
	return Advice{}
}

// #25: the suggestion formula must use the per-resource target, so a lower CPU
// target yields more CPU headroom and a higher memory target yields less
// memory headroom than the shared value would.
func TestRunUsesPerResourceTargets(t *testing.T) {
	now := time.Now().UTC()
	const cpuM = 700.0
	const memB = 700 * 1024 * 1024.0
	recs := splitFixtures(now, cpuM, memB, 40)

	cfg := config.Recommend{
		WindowDays: 7, Percentile: 95, MinSamples: 24,
		TargetUtilization:       0.70,
		CPUTargetUtilization:    0.50,
		MemoryTargetUtilization: 0.90,
	}
	res := Run(recs, cfg, now)

	if res.CPUTargetUtil != 0.50 || res.MemTargetUtil != 0.90 {
		t.Fatalf("result targets: cpu=%v mem=%v", res.CPUTargetUtil, res.MemTargetUtil)
	}

	a := splitAdvice(t, res)
	wantCPU := cpuM / 0.50
	if math.Abs(a.CPUSuggested-wantCPU) > 10 {
		t.Fatalf("CPUSuggested = %v, want ~%v (p/0.50)", a.CPUSuggested, wantCPU)
	}
	wantMem := memB / 0.90
	if math.Abs(float64(a.MemSuggested)-wantMem) > 1<<20 {
		t.Fatalf("MemSuggested = %v, want ~%v (p/0.90)", a.MemSuggested, wantMem)
	}

	// Same inputs against the shared target must produce different suggestions,
	// which is the whole point of splitting the knob.
	shared := Run(recs, config.Recommend{
		WindowDays: 7, Percentile: 95, MinSamples: 24, TargetUtilization: 0.70,
	}, now)
	sa := splitAdvice(t, shared)
	if a.CPUSuggested <= sa.CPUSuggested {
		t.Fatalf("lower CPU target must give more headroom: split=%v shared=%v", a.CPUSuggested, sa.CPUSuggested)
	}
	if a.MemSuggested >= sa.MemSuggested {
		t.Fatalf("higher memory target must give less headroom: split=%v shared=%v", a.MemSuggested, sa.MemSuggested)
	}
}

// The persisted snapshot must carry the resolved targets so --score can label
// against the target the suggestion was built with.
func TestSnapshotsCarryResolvedTargets(t *testing.T) {
	now := time.Now().UTC()
	recs := splitFixtures(now, 700, 700*1024*1024, 40)
	res := Run(recs, config.Recommend{
		WindowDays: 7, Percentile: 95, MinSamples: 24,
		TargetUtilization:       0.70,
		CPUTargetUtilization:    0.50,
		MemoryTargetUtilization: 0.90,
	}, now)

	snaps := SnapshotsFromResult(res, "c1", "cl1", now)
	if len(snaps) == 0 {
		t.Fatal("no snapshots")
	}
	s := snaps[0]
	if s.CPUTargetUtilization != 0.50 || s.MemoryTargetUtilization != 0.90 {
		t.Fatalf("snapshot targets: cpu=%v mem=%v", s.CPUTargetUtilization, s.MemoryTargetUtilization)
	}
	if s.CPUTarget() != 0.50 || s.MemTarget() != 0.90 {
		t.Fatalf("resolved: cpu=%v mem=%v", s.CPUTarget(), s.MemTarget())
	}
}

// Pre-#25 rows have both per-resource columns at 0 and must fall back.
func TestSnapshotTargetsFallBackForLegacyRows(t *testing.T) {
	legacy := store.Recommendation{TargetUtilization: 0.70}
	if legacy.CPUTarget() != 0.70 || legacy.MemTarget() != 0.70 {
		t.Fatalf("legacy fallback: cpu=%v mem=%v", legacy.CPUTarget(), legacy.MemTarget())
	}
}

// Scoring must label each resource against its own target: a memory target of
// 0.90 with the same observed utilization reads differently than 0.70.
func TestScoreLabelsAgainstPerResourceTarget(t *testing.T) {
	now := time.Now().UTC()
	recAt := now.Add(-time.Hour)

	newRec := store.Recommendation{
		Grain: "workload", GrainKey: "Deployment/web/app",
		Namespace: "ns", Container: "app", OwnerKind: "Deployment", OwnerName: "web",
		RecordedAt:   recAt,
		CPUSugMcores: 1400, MemSugBytes: 800 * 1024 * 1024,
		TargetUtilization:    0.70,
		CPUTargetUtilization: 0.50, MemoryTargetUtilization: 0.90,
	}
	// After the change, utilization settles at 0.65 of each suggestion.
	// Against the shared 0.70 target the band is 0.55..0.85, so 0.65 is a hit.
	// Against the memory target of 0.90 the band is 0.75..1.05, so 0.65 is a
	// miss-low — the same data, read correctly per resource.
	const ratio = 0.65
	samples := []store.Record{}
	for i := 0; i < 30; i++ {
		samples = append(samples, store.Record{
			SampledAt: now.Add(time.Duration(i) * time.Minute),
			Namespace: "ns", Pod: "web-abc", Container: "app",
			OwnerKind: "Deployment", OwnerName: "web",
			CPUUtilMcores: 1400 * ratio,
			MemUtilBytes:  int64(800 * 1024 * 1024 * ratio),
		})
	}

	splitRes := ScoreCalibration([]store.Recommendation{newRec}, samples, ScoreOptions{MinSamples: 24})
	if len(splitRes.Rows) != 1 {
		t.Fatalf("rows: %d", len(splitRes.Rows))
	}
	if got := splitRes.Rows[0].MemLabel; got != ScoreMissLo {
		t.Fatalf("memory label against the 0.90 target = %q, want %q", got, ScoreMissLo)
	}

	legacy := newRec
	legacy.CPUTargetUtilization, legacy.MemoryTargetUtilization = 0, 0
	legacyRes := ScoreCalibration([]store.Recommendation{legacy}, samples, ScoreOptions{MinSamples: 24})
	if got := legacyRes.Rows[0].MemLabel; got != ScoreHit {
		t.Fatalf("memory label against the shared 0.70 target = %q, want %q", got, ScoreHit)
	}
}
