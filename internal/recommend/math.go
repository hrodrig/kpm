package recommend

import (
	"math"
	"sort"
)

const (
	cpuStepMcores = 10.0
	memStepBytes  = 1024 * 1024 // 1 MiB
)

// Percentile returns the nearest-rank percentile of values (p in 0..100).
// Empty input returns 0.
func Percentile(values []float64, p int) float64 {
	if len(values) == 0 {
		return 0
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if p == 0 {
		return cp[0]
	}
	// nearest-rank: rank = ceil(p/100 * n)
	rank := int(math.Ceil(float64(p) / 100 * float64(len(cp))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(cp) {
		rank = len(cp)
	}
	return cp[rank-1]
}

// RoundUpCPUMillicores ceils to the next 10 millicores (minimum step).
func RoundUpCPUMillicores(m float64) float64 {
	if m <= 0 {
		return 0
	}
	return math.Ceil(m/cpuStepMcores) * cpuStepMcores
}

// RoundUpMemBytes ceils to the next 1 MiB.
func RoundUpMemBytes(b float64) int64 {
	if b <= 0 {
		return 0
	}
	return int64(math.Ceil(b/float64(memStepBytes))) * memStepBytes
}

// SuggestedRequest returns round_up(p / targetUtilization).
func SuggestedCPU(pMcores, targetUtilization float64) float64 {
	if targetUtilization <= 0 {
		return RoundUpCPUMillicores(pMcores)
	}
	return RoundUpCPUMillicores(pMcores / targetUtilization)
}

// SuggestedMem returns round_up(p / targetUtilization) in bytes.
func SuggestedMem(pBytes, targetUtilization float64) int64 {
	if targetUtilization <= 0 {
		return RoundUpMemBytes(pBytes)
	}
	return RoundUpMemBytes(pBytes / targetUtilization)
}

// Verdict is a rightsizing band label.
type Verdict string

const (
	VerdictOK         Verdict = "ok"
	VerdictOversized  Verdict = "oversized"
	VerdictUndersized Verdict = "undersized"
	VerdictNA         Verdict = "n/a"
)

// Classify compares utilPercentile/currentAllocation to oversize/undersize bands.
func Classify(utilP, current, oversizeRatio, undersizeRatio float64) Verdict {
	if current <= 0 {
		return VerdictNA
	}
	ratio := utilP / current
	if ratio < oversizeRatio {
		return VerdictOversized
	}
	if ratio > undersizeRatio {
		return VerdictUndersized
	}
	return VerdictOK
}
