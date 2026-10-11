package recommend

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/store"
)

func TestScoreCalibrationHitAndSkip(t *testing.T) {
	recAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	recs := []store.Recommendation{{
		Grain: "pod", GrainKey: "app/p/c",
		Namespace: "app", Pod: "p", Container: "c",
		RecordedAt: recAt, TargetUtilization: 0.70,
		CPUSugMcores: 100, MemSugBytes: 100 << 20,
	}}
	var samples []store.Record
	// util ≈ 70% of suggested → hit
	for i := 0; i < 30; i++ {
		samples = append(samples, store.Record{
			SampledAt: recAt.Add(time.Duration(i+1) * time.Hour),
			Namespace: "app", Pod: "p", Container: "c",
			CPUUtilMcores: 70, MemUtilBytes: 70 << 20,
		})
	}
	res := ScoreCalibration(recs, samples, ScoreOptions{MinSamples: 24, Percentile: 95, Band: 0.15})
	if res.Skipped != 0 || res.HitsCPU != 1 || res.HitsMem != 1 {
		t.Fatalf("want hits: %+v rows=%+v", res, res.Rows)
	}

	short := ScoreCalibration(recs, samples[:5], ScoreOptions{MinSamples: 24})
	if short.Skipped != 1 || short.Rows[0].CPULabel != ScoreSkip {
		t.Fatalf("want skip: %+v", short)
	}
}

func TestScoreMissHighAndFormat(t *testing.T) {
	recAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	recs := []store.Recommendation{{
		Grain: "workload", GrainKey: "Deployment/web/c",
		OwnerKind: "Deployment", OwnerName: "web", Container: "c",
		RecordedAt: recAt, TargetUtilization: 0.70,
		CPUSugMcores: 100, MemSugBytes: 100 << 20,
	}}
	var samples []store.Record
	for i := 0; i < 30; i++ {
		samples = append(samples, store.Record{
			SampledAt: recAt.Add(time.Duration(i+1) * time.Hour),
			OwnerKind: "Deployment", OwnerName: "web", Container: "c",
			CPUUtilMcores: 95, MemUtilBytes: 95 << 20, // ratio 0.95 → miss-high
		})
	}
	res := ScoreCalibration(recs, samples, ScoreOptions{MinSamples: 24, Band: 0.15})
	if res.Rows[0].CPULabel != ScoreMissHi {
		t.Fatalf("cpu label: %s", res.Rows[0].CPULabel)
	}
	var buf bytes.Buffer
	if err := FormatScoreTable(&buf, res); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "miss-high") || !strings.Contains(buf.String(), "cpu hits") {
		t.Fatalf("format: %q", buf.String())
	}
}

func TestLabelUtilEdge(t *testing.T) {
	if labelUtilVsSug(10, 0, 0.7, 0.15) != ScoreNoData {
		t.Fatal("no-data")
	}
	if labelUtilVsSug(10, 100, 0.7, 0.15) != ScoreMissLo {
		t.Fatal("miss-lo")
	}
}
