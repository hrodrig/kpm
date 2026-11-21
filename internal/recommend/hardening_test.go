package recommend

import (
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func TestWorkloadGrainSeparatesNamespaces(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	var recs []store.Record
	for i := 0; i < 30; i++ {
		at := now.Add(-time.Duration(i) * time.Hour)
		recs = append(recs,
			store.Record{
				SampledAt: at, Namespace: "staging", Pod: "api-0", Container: "app",
				OwnerKind: "Deployment", OwnerName: "api",
				CPUUtilMcores: 100, CPURequestMcores: 200,
				MemUtilBytes: 64 << 20, MemRequestBytes: 128 << 20,
			},
			store.Record{
				SampledAt: at, Namespace: "prod", Pod: "api-0", Container: "app",
				OwnerKind: "Deployment", OwnerName: "api",
				CPUUtilMcores: 700, CPURequestMcores: 1000,
				MemUtilBytes: 512 << 20, MemRequestBytes: 1024 << 20,
			},
		)
	}

	cfg := config.Recommend{WindowDays: 7, Percentile: 95, MinSamples: 24, TargetUtilization: 0.70}
	res := Run(recs, cfg, now)

	byKey := map[string]Advice{}
	for _, a := range res.Advice {
		if a.Grain == GrainWorkload {
			byKey[a.Key] = a
		}
	}
	stg, okStg := byKey["staging/Deployment/api/app"]
	prod, okProd := byKey["prod/Deployment/api/app"]
	if !okStg || !okProd {
		t.Fatalf("expected separate namespace workload keys, got %+v", byKey)
	}
	if stg.CPUPercentile != 100 || stg.Namespace != "staging" {
		t.Fatalf("staging polluted: %+v", stg)
	}
	if prod.CPUPercentile != 700 || prod.Namespace != "prod" {
		t.Fatalf("prod polluted: %+v", prod)
	}
}

func TestScoreCalibrationNamespacesAndStoredPercentile(t *testing.T) {
	recAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	recs := []store.Recommendation{
		{
			Grain: "workload", GrainKey: "staging/Deployment/api/app",
			Namespace: "staging", OwnerKind: "Deployment", OwnerName: "api", Container: "app",
			RecordedAt: recAt, Percentile: 50, TargetUtilization: 0.70,
			CPUSugMcores: 100, MemSugBytes: 100 << 20,
		},
		{
			Grain: "workload", GrainKey: "prod/Deployment/api/app",
			Namespace: "prod", OwnerKind: "Deployment", OwnerName: "api", Container: "app",
			RecordedAt: recAt, Percentile: 95, TargetUtilization: 0.70,
			CPUSugMcores: 1000, MemSugBytes: 1000 << 20,
		},
	}

	var samples []store.Record
	// 25 samples for staging: 24 at 70m, 1 outlier at 500m -> p50=70m (hit), p95=500m (miss-high).
	// 25 samples for prod: all at 700m -> p95=700m (hit).
	for i := 0; i < 25; i++ {
		at := recAt.Add(time.Duration(i+1) * time.Hour)
		stgCPU := 70.0
		if i == 24 {
			stgCPU = 500.0
		}
		samples = append(samples,
			store.Record{
				SampledAt: at, Namespace: "staging", Pod: "api-0", Container: "app",
				OwnerKind: "Deployment", OwnerName: "api",
				CPUUtilMcores: stgCPU, MemUtilBytes: 70 << 20,
			},
			store.Record{
				SampledAt: at, Namespace: "prod", Pod: "api-0", Container: "app",
				OwnerKind: "Deployment", OwnerName: "api",
				CPUUtilMcores: 700, MemUtilBytes: 700 << 20,
			},
		)
	}

	res := ScoreCalibration(recs, samples, ScoreOptions{MinSamples: 24, Percentile: 95, Band: 0.15})
	if res.Skipped != 0 || res.HitsCPU != 2 || res.HitsMem != 2 {
		t.Fatalf("expected both namespaces to hit using stored percentiles, got %+v rows=%+v", res, res.Rows)
	}
}
