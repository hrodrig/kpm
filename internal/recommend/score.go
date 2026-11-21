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

	idx := indexSamplesForScore(samples)
	out := ScoreResult{Band: opt.Band, MinSamples: opt.MinSamples}
	for _, rec := range recs {
		row := scoreOneRec(rec, idx, opt)
		if row.Samples < opt.MinSamples {
			out.Skipped++
		} else {
			tallyScoreRow(&out, row)
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

type scoreSampleIndex struct {
	byPod      map[string][]store.Record
	byWorkload map[string][]store.Record
}

func indexSamplesForScore(samples []store.Record) scoreSampleIndex {
	idx := scoreSampleIndex{
		byPod:      make(map[string][]store.Record),
		byWorkload: make(map[string][]store.Record),
	}
	for _, s := range samples {
		pk := s.Namespace + "/" + s.Pod + "/" + s.Container
		idx.byPod[pk] = append(idx.byPod[pk], s)
		if s.OwnerKind != "" && s.OwnerName != "" {
			wk := WorkloadGrainKey(s.Namespace, s.OwnerKind, s.OwnerName, s.Container)
			idx.byWorkload[wk] = append(idx.byWorkload[wk], s)
		}
	}
	return idx
}

func scoreOneRec(rec store.Recommendation, idx scoreSampleIndex, opt ScoreOptions) ScoreRow {
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
	cpuVals, memVals := collectAfterIndexed(idx, rec)
	row.Samples = len(cpuVals)
	if row.Samples < opt.MinSamples {
		row.CPULabel = ScoreSkip
		row.MemLabel = ScoreSkip
		return row
	}
	pct := rec.Percentile
	if pct < 1 {
		pct = opt.Percentile
	}
	row.CPUAfterP = Percentile(cpuVals, pct)
	row.MemAfterP = Percentile(memVals, pct)
	row.CPULabel = labelUtilVsSug(row.CPUAfterP, rec.CPUSugMcores, rec.CPUTarget(), opt.Band)
	row.MemLabel = labelUtilVsSug(row.MemAfterP, float64(rec.MemSugBytes), rec.MemTarget(), opt.Band)
	return row
}

func tallyScoreRow(out *ScoreResult, row ScoreRow) {
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
}

func collectAfterIndexed(idx scoreSampleIndex, rec store.Recommendation) (cpu, mem []float64) {
	var bucket []store.Record
	switch Grain(rec.Grain) {
	case GrainWorkload:
		bucket = idx.byWorkload[WorkloadGrainKey(rec.Namespace, rec.OwnerKind, rec.OwnerName, rec.Container)]
	default:
		bucket = idx.byPod[rec.Namespace+"/"+rec.Pod+"/"+rec.Container]
	}
	cutoff := rec.RecordedAt.UTC()
	for _, s := range bucket {
		if !s.SampledAt.UTC().After(cutoff) {
			continue
		}
		cpu = append(cpu, s.CPUUtilMcores)
		mem = append(mem, float64(s.MemUtilBytes))
	}
	return cpu, mem
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
