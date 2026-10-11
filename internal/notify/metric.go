package notify

import (
	"strings"

	"github.com/hrodrig/kpm/internal/store"
)

// MetricValue returns the observed value for a rule metric and whether it is usable.
func MetricValue(metric string, rec store.Record) (value float64, ok bool) {
	switch strings.TrimSpace(metric) {
	case metricCPUUtilReq:
		return ratio(rec.CPUUtilMcores, rec.CPURequestMcores)
	case metricMemUtilReq:
		return ratio(float64(rec.MemUtilBytes), float64(rec.MemRequestBytes))
	case metricCPUUtilLim:
		return ratio(rec.CPUUtilMcores, rec.CPULimitMcores)
	case metricMemUtilLim:
		return ratio(float64(rec.MemUtilBytes), float64(rec.MemLimitBytes))
	case metricCPUUtilAbs:
		if rec.CPUUtilMcores < 0 {
			return 0, false
		}
		return rec.CPUUtilMcores, true
	case metricMemUtilAbs:
		if rec.MemUtilBytes < 0 {
			return 0, false
		}
		return float64(rec.MemUtilBytes), true
	default:
		return 0, false
	}
}

func ratio(util, denom float64) (float64, bool) {
	if denom <= 0 {
		return 0, false
	}
	return util / denom, true
}

// Breach reports whether value meets the rule threshold.
func Breach(metric string, thresholdRatio, threshold, value float64) bool {
	switch strings.TrimSpace(metric) {
	case metricCPUUtilAbs, metricMemUtilAbs:
		return value >= threshold
	default:
		return value >= thresholdRatio
	}
}

// ThresholdForDisplay returns the configured threshold number for payloads.
func ThresholdForDisplay(metric string, thresholdRatio, threshold float64) float64 {
	switch strings.TrimSpace(metric) {
	case metricCPUUtilAbs, metricMemUtilAbs:
		return threshold
	default:
		return thresholdRatio
	}
}
