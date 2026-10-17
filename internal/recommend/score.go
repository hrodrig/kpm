package recommend

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/hrodrig/kpm/internal/store"
)

// ScoreLabel is a calibration outcome vs target utilization.
type ScoreLabel string

const (
	ScoreHit    ScoreLabel = "hit"
	ScoreMissHi ScoreLabel = "miss-high"
	ScoreMissLo ScoreLabel = "miss-low"
	ScoreSkip   ScoreLabel = "skip"
	ScoreNoData ScoreLabel = "no-data"
)

// ScoreRow is one recommendation evaluated against later samples.
type ScoreRow struct {
	Grain         string
	GrainKey      string
	RecordedAt    time.Time
	Samples       int
	CPUSug        float64
	MemSug        int64
	CPUAfterP     float64
	MemAfterP     float64
	CPULabel      ScoreLabel
	MemLabel      ScoreLabel
	CPUTargetUtil float64
	MemTargetUtil float64
	Band          float64
}

// ScoreResult aggregates calibration rows.
type ScoreResult struct {
	Rows       []ScoreRow
	HitsCPU    int
	HitsMem    int
	ScoredCPU  int
	ScoredMem  int
	Skipped    int
	Band       float64
	MinSamples int
}

// ScoreOptions configures calibration scoring.
type ScoreOptions struct {
	MinSamples int
	Percentile int
	Band       float64 // absolute slack around target util (default 0.15)
}

// ScoreCalibration compares post-recommend util percentiles to suggested requests
// (mode A: assume operator applied suggestions).
func ScoreCalibration(recs []store.Recommendation, samples []store.Record, opt ScoreOptions) ScoreResult {
	if opt.MinSamples < 1 {
		opt.MinSamples = 24
	}
	if opt.Percentile < 1 {
		opt.Percentile = 95
	}
	if opt.Band <= 0 {
		opt.Band = 0.15
	}

	out := ScoreResult{Band: opt.Band, MinSamples: opt.MinSamples}
	for _, rec := range recs {
		row := ScoreRow{
			Grain:         rec.Grain,
			GrainKey:      rec.GrainKey,
			RecordedAt:    rec.RecordedAt,
			CPUSug:        rec.CPUSugMcores,
			MemSug:        rec.MemSugBytes,
			CPUTargetUtil: rec.CPUTarget(),
			MemTargetUtil: rec.MemTarget(),
			Band:          opt.Band,
		}
		cpuVals, memVals := collectAfter(samples, rec)
		row.Samples = len(cpuVals)
		if row.Samples < opt.MinSamples {
			row.CPULabel = ScoreSkip
			row.MemLabel = ScoreSkip
			out.Skipped++
			out.Rows = append(out.Rows, row)
			continue
		}
		row.CPUAfterP = Percentile(cpuVals, opt.Percentile)
		row.MemAfterP = Percentile(memVals, opt.Percentile)
		row.CPULabel = labelUtilVsSug(row.CPUAfterP, rec.CPUSugMcores, rec.CPUTarget(), opt.Band)
		row.MemLabel = labelUtilVsSug(row.MemAfterP, float64(rec.MemSugBytes), rec.MemTarget(), opt.Band)
		if row.CPULabel != ScoreNoData {
			out.ScoredCPU++
			if row.CPULabel == ScoreHit {
				out.HitsCPU++
			}
		}
		if row.MemLabel != ScoreNoData {
			out.ScoredMem++
			if row.MemLabel == ScoreHit {
				out.HitsMem++
			}
		}
		out.Rows = append(out.Rows, row)
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		if out.Rows[i].Grain != out.Rows[j].Grain {
			return out.Rows[i].Grain < out.Rows[j].Grain
		}
		return out.Rows[i].GrainKey < out.Rows[j].GrainKey
	})
	return out
}

func collectAfter(samples []store.Record, rec store.Recommendation) (cpu, mem []float64) {
	cutoff := rec.RecordedAt.UTC()
	for _, s := range samples {
		at := s.SampledAt.UTC()
		if !at.After(cutoff) {
			continue
		}
		if !matchGrain(s, rec) {
			continue
		}
		cpu = append(cpu, s.CPUUtilMcores)
		mem = append(mem, float64(s.MemUtilBytes))
	}
	return cpu, mem
}

func matchGrain(s store.Record, rec store.Recommendation) bool {
	switch Grain(rec.Grain) {
	case GrainWorkload:
		return s.OwnerKind == rec.OwnerKind && s.OwnerName == rec.OwnerName && s.Container == rec.Container
	default:
		return s.Namespace == rec.Namespace && s.Pod == rec.Pod && s.Container == rec.Container
	}
}

func labelUtilVsSug(utilP, sug, target, band float64) ScoreLabel {
	if sug <= 0 {
		return ScoreNoData
	}
	ratio := utilP / sug
	lo := target - band
	hi := target + band
	if lo < 0 {
		lo = 0
	}
	if ratio < lo {
		return ScoreMissLo
	}
	if ratio > hi {
		return ScoreMissHi
	}
	return ScoreHit
}

// FormatScoreTable writes a compact score summary table.
func FormatScoreTable(w io.Writer, res ScoreResult) error {
	if _, err := fmt.Fprintf(w, "GRAIN\tKEY\tSAMPLES\tCPU_P/SUG\tCPU\tMEM_P/SUG\tMEM\n"); err != nil {
		return err
	}
	for _, r := range res.Rows {
		cpuRatio := "n/a"
		if r.CPUSug > 0 && r.CPULabel != ScoreSkip && r.CPULabel != ScoreNoData {
			cpuRatio = fmt.Sprintf("%.2f", r.CPUAfterP/r.CPUSug)
		}
		memRatio := "n/a"
		if r.MemSug > 0 && r.MemLabel != ScoreSkip && r.MemLabel != ScoreNoData {
			memRatio = fmt.Sprintf("%.2f", r.MemAfterP/float64(r.MemSug))
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			r.Grain, r.GrainKey, r.Samples, cpuRatio, r.CPULabel, memRatio, r.MemLabel); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "# cpu hits %d/%d mem hits %d/%d skipped %d band ±%.2f\n",
		res.HitsCPU, res.ScoredCPU, res.HitsMem, res.ScoredMem, res.Skipped, res.Band)
	return err
}
