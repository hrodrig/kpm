package recommend

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func TestPercentileAndRoundUp(t *testing.T) {
	vals := []float64{10, 20, 30, 40, 50}
	if p := Percentile(vals, 95); p != 50 {
		t.Fatalf("p95: %v", p)
	}
	if p := Percentile(vals, 50); p != 30 {
		t.Fatalf("p50: %v", p)
	}
	if RoundUpCPUMillicores(11) != 20 {
		t.Fatal("cpu step")
	}
	if RoundUpMemBytes(1) != 1024*1024 {
		t.Fatal("mem step")
	}
	if SuggestedCPU(70, 0.70) != 100 {
		t.Fatalf("suggest cpu: %v", SuggestedCPU(70, 0.70))
	}
}

func TestClassify(t *testing.T) {
	if Classify(20, 100, 0.35, 0.85) != VerdictOversized {
		t.Fatal("oversized")
	}
	if Classify(90, 100, 0.35, 0.85) != VerdictUndersized {
		t.Fatal("undersized")
	}
	if Classify(50, 100, 0.35, 0.85) != VerdictOK {
		t.Fatal("ok")
	}
	if Classify(10, 0, 0.35, 0.85) != VerdictNA {
		t.Fatal("n/a")
	}
}

func TestRunDualGrain(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	cfg := config.Recommend{
		WindowDays: 7, Percentile: 95, MinSamples: 3,
		TargetUtilization: 0.70,
		CPUOversizeRatio:  0.35, CPUUndersizeRatio: 0.85,
		CPULimitOversizeRatio: 0.35, CPULimitUndersizeRatio: 0.85,
		MemoryOversizeRatio: 0.35, MemoryUndersizeRatio: 0.85,
		MemoryLimitOversizeRatio: 0.35, MemoryLimitUndersizeRatio: 0.85,
	}
	var recs []store.Record
	for i := 0; i < 5; i++ {
		recs = append(recs, store.Record{
			SampledAt: now.Add(-time.Duration(i) * time.Hour),
			Namespace: "app", Pod: "web-0", Container: "c",
			OwnerKind: "ReplicaSet", OwnerName: "web-abc",
			CPUUtilMcores: 40, MemUtilBytes: 16 << 20,
			CPURequestMcores: 200, MemRequestBytes: 128 << 20,
			CPULimitMcores: 400, MemLimitBytes: 256 << 20,
		})
	}
	res := Run(recs, cfg, now)
	if len(res.Advice) != 2 {
		t.Fatalf("want workload+pod advice, got %d", len(res.Advice))
	}
	var sawW, sawP bool
	for _, a := range res.Advice {
		if a.Grain == GrainWorkload {
			sawW = true
		}
		if a.Grain == GrainPod {
			sawP = true
		}
		if a.CPUSuggested != 60 { // 40/0.7=57.14 → 60
			t.Fatalf("cpu sug: %v", a.CPUSuggested)
		}
		if a.CPUVerdictReq != VerdictOversized {
			t.Fatalf("verdict: %s", a.CPUVerdictReq)
		}
	}
	if !sawW || !sawP {
		t.Fatal("dual grain missing")
	}
}

func TestRunSkipsLowSamples(t *testing.T) {
	now := time.Now().UTC()
	cfg := config.Recommend{
		WindowDays: 7, Percentile: 95, MinSamples: 10,
		TargetUtilization: 0.70,
		CPUOversizeRatio:  0.35, CPUUndersizeRatio: 0.85,
		CPULimitOversizeRatio: 0.35, CPULimitUndersizeRatio: 0.85,
		MemoryOversizeRatio: 0.35, MemoryUndersizeRatio: 0.85,
		MemoryLimitOversizeRatio: 0.35, MemoryLimitUndersizeRatio: 0.85,
	}
	recs := []store.Record{{
		SampledAt: now, Namespace: "a", Pod: "p", Container: "c",
		CPUUtilMcores: 1, CPURequestMcores: 100,
	}}
	res := Run(recs, cfg, now)
	if len(res.Advice) != 0 || res.SkippedKeys == 0 {
		t.Fatalf("advice=%d skipped=%d", len(res.Advice), res.SkippedKeys)
	}
}

func TestFormatTableAndTSV(t *testing.T) {
	a := []Advice{{
		Grain: GrainPod, Key: "app/web/c", Samples: 24,
		CPUPercentile: 40, CPURequest: 100, CPUSuggested: 60, CPUVerdictReq: VerdictOversized,
		MemPercentile: 16 << 20, MemRequest: 64 << 20, MemSuggested: 32 << 20, MemVerdictReq: VerdictOversized,
	}}
	var buf bytes.Buffer
	if err := FormatTable(&buf, a); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "GRAIN") || !strings.Contains(buf.String(), "app/web/c") {
		t.Fatalf("table: %q", buf.String())
	}
	buf.Reset()
	if err := FormatTSV(&buf, a); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "grain\tkey") {
		t.Fatalf("tsv: %q", buf.String())
	}
}
