package recommend

import (
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func coverageCfg() config.Recommend {
	return config.Recommend{
		WindowDays: 7, Percentile: 95, MinSamples: 3,
		TargetUtilization: 0.70,
		CPUOversizeRatio:  0.35, CPUUndersizeRatio: 0.85,
		CPULimitOversizeRatio: 0.35, CPULimitUndersizeRatio: 0.85,
		MemoryOversizeRatio: 0.35, MemoryUndersizeRatio: 0.85,
		MemoryLimitOversizeRatio: 0.35, MemoryLimitUndersizeRatio: 0.85,
	}
}

func coverageRec(at time.Time) store.Record {
	return store.Record{
		SampledAt:     at,
		Namespace:     "app",
		Pod:           "p",
		Container:     "c",
		OwnerKind:     "Deployment",
		OwnerName:     "web",
		CPUUtilMcores: 10,
		MemUtilBytes:  1 << 20,
	}
}

// #26: the window says what was asked for; DaysWithSamples says what the
// percentiles actually rest on.
func TestRunWindowCoverage(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cfg := coverageCfg()

	every := func(from, to time.Time, step time.Duration) []store.Record {
		recs := []store.Record{}
		for at := from; !at.After(to); at = at.Add(step) {
			recs = append(recs, coverageRec(at))
		}
		return recs
	}
	var noTime time.Time

	tests := []struct {
		name       string
		records    []store.Record
		wantDays   int
		wantOldest time.Time
		wantNewest time.Time
	}{
		{
			// Twice a day for the full span: the rolling window clips the first
			// and last day, so full coverage is WindowDays+1 distinct UTC dates.
			name:       "contiguous full coverage",
			records:    every(now.AddDate(0, 0, -7), now, 12*time.Hour),
			wantDays:   cfg.WindowDays + 1,
			wantOldest: now.AddDate(0, 0, -7),
			wantNewest: now,
		},
		{
			name: "gap: only the last three days hold samples",
			records: []store.Record{
				coverageRec(now.AddDate(0, 0, -2)),
				coverageRec(now.AddDate(0, 0, -1)),
				coverageRec(now),
			},
			wantDays:   3,
			wantOldest: now.AddDate(0, 0, -2),
			wantNewest: now,
		},
		{
			name:     "no samples inside the window",
			records:  []store.Record{coverageRec(now.AddDate(0, 0, -30))},
			wantDays: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Run(tt.records, cfg, now)

			if res.WindowDays != cfg.WindowDays {
				t.Errorf("window_days: got %d want %d", res.WindowDays, cfg.WindowDays)
			}
			if res.DaysWithSamples != tt.wantDays {
				t.Errorf("days_with_samples: got %d want %d", res.DaysWithSamples, tt.wantDays)
			}
			if !res.OldestSample.Equal(tt.wantOldest) {
				t.Errorf("oldest sample: got %v want %v", res.OldestSample, tt.wantOldest)
			}
			if !res.NewestSample.Equal(tt.wantNewest) {
				t.Errorf("newest sample: got %v want %v", res.NewestSample, tt.wantNewest)
			}
			if tt.wantDays == 0 && !res.OldestSample.Equal(noTime) {
				t.Errorf("empty window should leave the sample bounds zeroed")
			}
		})
	}
}

// Repeated samples on one day are one covered day, not one per sample.
func TestRunWindowCoverageCountsDaysNotSamples(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	day := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)

	records := []store.Record{}
	for i := range 12 {
		records = append(records, coverageRec(day.Add(time.Duration(i)*time.Hour)))
	}
	res := Run(records, coverageCfg(), now)

	if res.RecordsInWin != len(records) {
		t.Fatalf("rows in window: got %d want %d", res.RecordsInWin, len(records))
	}
	if res.DaysWithSamples != 1 {
		t.Fatalf("days with samples: got %d want 1", res.DaysWithSamples)
	}
}

// Coverage reaches the persisted snapshot so the API can expose it.
func TestSnapshotsFromResultCarryCoverage(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	records := []store.Record{}
	for i := range 3 {
		at := now.AddDate(0, 0, -i)
		for j := range 3 {
			records = append(records, coverageRec(at.Add(time.Duration(j)*time.Minute)))
		}
	}
	res := Run(records, coverageCfg(), now)
	if res.DaysWithSamples != 3 {
		t.Fatalf("precondition: days with samples = %d", res.DaysWithSamples)
	}

	snaps := SnapshotsFromResult(res, "client", "cluster", now)
	if len(snaps) == 0 {
		t.Fatal("no snapshots from result")
	}
	for _, s := range snaps {
		if s.DaysWithSamples != res.DaysWithSamples {
			t.Fatalf("snapshot coverage: got %d want %d", s.DaysWithSamples, res.DaysWithSamples)
		}
	}
}
