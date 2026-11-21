package recommend

import (
	"math"
	"sort"
)

const (
	cpuStepMcores = 10.0
	memStepBytes  = 1024 * 1024 // 1 MiB
	ceilEpsilon   = 1e-9
)

func ceilWithEps(v float64) float64 {
	f := math.Floor(v)
	if v-f <= ceilEpsilon {
		return f
	}
	return math.Ceil(v)
}

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
	if math.IsNaN(m) || math.IsInf(m, 0) || m <= 0 {
		return 0
	}
	return ceilWithEps(m/cpuStepMcores) * cpuStepMcores
}

// RoundUpMemBytes ceils to the next 1 MiB, clamping NaN/Inf/<=0 to 0 and
// preventing int64 wrap-around on extreme inputs.
func RoundUpMemBytes(b float64) int64 {
	if math.IsNaN(b) || math.IsInf(b, 0) || b <= 0 {
		return 0
	}
	steps := ceilWithEps(b / float64(memStepBytes))
	const maxSteps = math.MaxInt64 / memStepBytes
	if steps >= float64(maxSteps) {
		return maxSteps * memStepBytes
	}
	return int64(steps) * memStepBytes
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

// DefaultOversizeRatio / DefaultUndersizeRatio match configs/kpm.sample.yml recommend:.
const (
	DefaultOversizeRatio  = 0.35
	DefaultUndersizeRatio = 0.85
)

// SnapshotVerdicts classifies CPU/mem request sizing from a persisted recommend row
// (p-util vs current request). Empty ratios use defaults.
func SnapshotVerdicts(cpuP, cpuReq float64, memP, memReq int64, oversizeRatio, undersizeRatio float64) (cpu, mem Verdict) {
	if oversizeRatio <= 0 {
		oversizeRatio = DefaultOversizeRatio
	}
	if undersizeRatio <= 0 {
		undersizeRatio = DefaultUndersizeRatio
	}
	return Classify(cpuP, cpuReq, oversizeRatio, undersizeRatio),
		Classify(float64(memP), float64(memReq), oversizeRatio, undersizeRatio)
}
