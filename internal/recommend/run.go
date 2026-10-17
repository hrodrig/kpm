package recommend

import (
	"fmt"
	"sort"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

// Grain identifies the aggregation key.
type Grain string

const (
	GrainWorkload Grain = "workload"
	GrainPod      Grain = "pod"
)

// Advice is one rightsizing row.
type Advice struct {
	Grain     Grain
	Key       string // kind/name/container or ns/pod/container
	Namespace string
	Pod       string
	Container string
	OwnerKind string
	OwnerName string
	Samples   int

	CPUPercentile   float64
	MemPercentile   float64
	CPURequest      float64
	MemRequest      int64
	CPULimit        float64
	MemLimit        int64
	CPUSuggested    float64
	MemSuggested    int64
	CPUVerdictReq   Verdict
	MemVerdictReq   Verdict
	CPUVerdictLimit Verdict
	MemVerdictLimit Verdict
}

// Result is the outcome of a recommend pass.
type Result struct {
	Advice        []Advice
	SkippedKeys   int
	WindowStart   time.Time
	WindowEnd     time.Time
	MinSamples    int
	Percentile    int
	TargetUtil    float64
	CPUTargetUtil float64
	MemTargetUtil float64
	RecordsInWin  int

	// WindowDays is the window that was asked for. DaysWithSamples counts the
	// distinct UTC calendar days inside it that hold at least one sample, so the
	// percentiles can be read against the evidence behind them rather than against
	// the requested window alone. A fully covered rolling window touches
	// WindowDays+1 calendar days: the window clips the first and the last day.
	WindowDays      int
	DaysWithSamples int
	OldestSample    time.Time
	NewestSample    time.Time
}

// ErrInsufficient is returned when no advice rows meet min_samples.
var ErrInsufficient = fmt.Errorf("insufficient samples for recommend")

// Run computes dual-grain advice from records. Window is [now-windowDays, now].
func Run(records []store.Record, cfg config.Recommend, now time.Time) Result {
	end := now.UTC()
	start := end.AddDate(0, 0, -cfg.WindowDays)
	res := Result{
		WindowStart:   start,
		WindowEnd:     end,
		MinSamples:    cfg.MinSamples,
		Percentile:    cfg.Percentile,
		TargetUtil:    cfg.TargetUtilization,
		CPUTargetUtil: cfg.CPUUtilTarget(),
		MemTargetUtil: cfg.MemUtilTarget(),
		WindowDays:    cfg.WindowDays,
	}

	inWin, cov := windowRecords(records, start, end)
	res.RecordsInWin = len(inWin)
	res.DaysWithSamples = cov.Days
	res.OldestSample = cov.Oldest
	res.NewestSample = cov.Newest

	type bucket struct {
		cpuUtil, memUtil []float64
		last             store.Record
	}
	workload := map[string]*bucket{}
	podGrain := map[string]*bucket{}

	add := func(m map[string]*bucket, key string, r store.Record) {
		b := m[key]
		if b == nil {
			b = &bucket{}
			m[key] = b
		}
		b.cpuUtil = append(b.cpuUtil, r.CPUUtilMcores)
		b.memUtil = append(b.memUtil, float64(r.MemUtilBytes))
		if r.SampledAt.After(b.last.SampledAt) || b.last.SampledAt.IsZero() {
			b.last = r
		}
	}

	for _, r := range inWin {
		add(podGrain, r.Namespace+"/"+r.Pod+"/"+r.Container, r)
		if r.OwnerKind != "" && r.OwnerName != "" {
			add(workload, r.OwnerKind+"/"+r.OwnerName+"/"+r.Container, r)
		}
	}

	build := func(grain Grain, m map[string]*bucket) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b := m[k]
			n := len(b.cpuUtil)
			if n < cfg.MinSamples {
				res.SkippedKeys++
				continue
			}
			cpuP := Percentile(b.cpuUtil, cfg.Percentile)
			memP := Percentile(b.memUtil, cfg.Percentile)
			last := b.last
			res.Advice = append(res.Advice, Advice{
				Grain:           grain,
				Key:             k,
				Namespace:       last.Namespace,
				Pod:             last.Pod,
				Container:       last.Container,
				OwnerKind:       last.OwnerKind,
				OwnerName:       last.OwnerName,
				Samples:         n,
				CPUPercentile:   cpuP,
				MemPercentile:   memP,
				CPURequest:      last.CPURequestMcores,
				MemRequest:      last.MemRequestBytes,
				CPULimit:        last.CPULimitMcores,
				MemLimit:        last.MemLimitBytes,
				CPUSuggested:    SuggestedCPU(cpuP, res.CPUTargetUtil),
				MemSuggested:    SuggestedMem(memP, res.MemTargetUtil),
				CPUVerdictReq:   Classify(cpuP, last.CPURequestMcores, cfg.CPUOversizeRatio, cfg.CPUUndersizeRatio),
				MemVerdictReq:   Classify(memP, float64(last.MemRequestBytes), cfg.MemoryOversizeRatio, cfg.MemoryUndersizeRatio),
				CPUVerdictLimit: Classify(cpuP, last.CPULimitMcores, cfg.CPULimitOversizeRatio, cfg.CPULimitUndersizeRatio),
				MemVerdictLimit: Classify(memP, float64(last.MemLimitBytes), cfg.MemoryLimitOversizeRatio, cfg.MemoryLimitUndersizeRatio),
			})
		}
	}

	build(GrainWorkload, workload)
	build(GrainPod, podGrain)
	return res
}

// coverage is how much of the requested window the samples actually span (#26).
type coverage struct {
	Days   int
	Oldest time.Time
	Newest time.Time
}

// windowRecords selects the records inside [start, end] and summarises how much
// of that window they cover. Days counts distinct UTC days holding at least one
// sample, so repeated samples on one day count once. A fully covered rolling
// window reports one day more than the window length, because the window clips
// the first and the last day.
func windowRecords(records []store.Record, start, end time.Time) ([]store.Record, coverage) {
	var inWin []store.Record
	seen := map[string]struct{}{}
	cov := coverage{}
	for _, r := range records {
		at := r.SampledAt.UTC()
		if at.Before(start) || at.After(end) {
			continue
		}
		inWin = append(inWin, r)
		seen[at.Format("2006-01-02")] = struct{}{}
		if cov.Oldest.IsZero() || at.Before(cov.Oldest) {
			cov.Oldest = at
		}
		if at.After(cov.Newest) {
			cov.Newest = at
		}
	}
	cov.Days = len(seen)
	return inWin, cov
}
