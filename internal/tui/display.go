package tui

import (
	"fmt"
	"strings"

	"github.com/hrodrig/kpm/internal/live"
)

// Metric is which resource drives the heatmap bar.
type Metric int

const (
	MetricCPU Metric = iota
	MetricMem
)

// Baseline is request or limit for ratio.
type Baseline int

const (
	BaselineRequest Baseline = iota
	BaselineLimit
)

// ParseBaseline maps config baseline string.
func ParseBaseline(s string) Baseline {
	if strings.EqualFold(strings.TrimSpace(s), "limit") {
		return BaselineLimit
	}
	return BaselineRequest
}

// DisplayRow is one rendered TUI line input.
type DisplayRow struct {
	Namespace string
	Pod       string
	Ratio     float64
	HasRatio  bool
	Delta     PodDelta
}

func podRatio(p live.PodRow, metric Metric, baseline Baseline) (float64, bool) {
	if metric == MetricMem {
		if baseline == BaselineLimit {
			return p.MemRatioLimit()
		}
		return p.MemRatioRequest()
	}
	if baseline == BaselineLimit {
		return p.CPURatioLimit()
	}
	return p.CPURatioRequest()
}

func effectiveSortKey(key live.SortKey, metric Metric) live.SortKey {
	switch key {
	case live.SortName, live.SortCPUUtil, live.SortMemUtil, live.SortMemRatio, live.SortCPURatio:
		return key
	default:
		if metric == MetricMem {
			return live.SortMemRatio
		}
		return live.SortCPURatio
	}
}

// CycleSort returns the next sort key in the operator cycle.
func CycleSort(cur live.SortKey) live.SortKey {
	order := []live.SortKey{
		live.SortCPURatio, live.SortMemRatio, live.SortCPUUtil, live.SortMemUtil, live.SortName,
	}
	for i, k := range order {
		if k == cur {
			return order[(i+1)%len(order)]
		}
	}
	return live.SortCPURatio
}

// PrepareRows builds sorted display rows for the current metric/baseline.
func PrepareRows(snap live.Snapshot, idx RecIndex, key live.SortKey, metric Metric, baseline Baseline) []DisplayRow {
	pods := live.AggregatePods(snap.Containers)
	live.SortPods(pods, effectiveSortKey(key, metric))
	out := make([]DisplayRow, 0, len(pods))
	for _, p := range pods {
		r, ok := podRatio(p, metric, baseline)
		out = append(out, DisplayRow{
			Namespace: p.Namespace,
			Pod:       p.Pod,
			Ratio:     r,
			HasRatio:  ok,
			Delta:     DeltaForPod(p, snap.Containers, idx),
		})
	}
	return out
}

// FormatIdentity returns namespace/pod truncated to max.
func FormatIdentity(ns, pod string, max int) string {
	s := ns + "/" + pod
	if max < 8 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// MetricLabel returns short metric label.
func MetricLabel(m Metric) string {
	if m == MetricMem {
		return "mem"
	}
	return "cpu"
}

// BaselineLabel returns short baseline label.
func BaselineLabel(b Baseline) string {
	if b == BaselineLimit {
		return "limit"
	}
	return "request"
}

// HeaderLine builds the status header. version is e.g. "v0.2.0" (from ldflags).
func HeaderLine(version string, n int, metric Metric, baseline Baseline, sort live.SortKey, showDelta bool, errMsg string) string {
	delta := "off"
	if showDelta {
		delta = "on"
	}
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	line := fmt.Sprintf("kpm %s  pods=%d  %s/%s  sort=%s  delta=%s  [j/k] [s]ort [m]etric [b]aseline [d]elta [q]uit",
		version, n, MetricLabel(metric), BaselineLabel(baseline), sort, delta)
	if errMsg != "" {
		line += "  ERR: " + errMsg
	}
	return line
}
