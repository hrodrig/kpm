// Package notify evaluates sustained threshold rules and fans out alerts.
package notify

import (
	"fmt"
	"strings"

	"github.com/hrodrig/kpm/internal/config"
)

const (
	defaultHoldSeconds   = 600
	defaultRepeatSeconds = 3600

	metricCPUUtilReq = "cpu.util_vs_request"
	metricMemUtilReq = "mem.util_vs_request"
	metricCPUUtilLim = "cpu.util_vs_limit"
	metricMemUtilLim = "mem.util_vs_limit"
	metricCPUUtilAbs = "cpu.util_mcores"
	metricMemUtilAbs = "mem.util_bytes"
)

// BuiltinDefaultsLog is the one-time English startup message (SPEC §6.0).
const BuiltinDefaultsLog = "notification rules not defined in user config; using built-in defaults: CPU and memory util vs request at 80%"

// EffectiveRules returns user rules, or built-in 80% defaults when empty.
// usedBuiltins is true when defaults were applied.
func EffectiveRules(n config.Notifications) (rules []config.NotifyRule, usedBuiltins bool) {
	if len(n.Rules) > 0 {
		out := make([]config.NotifyRule, len(n.Rules))
		for i := range n.Rules {
			out[i] = normalizeRule(n.Rules[i])
		}
		return out, false
	}
	return []config.NotifyRule{
		normalizeRule(config.NotifyRule{
			Name:           "default-cpu-util-vs-request",
			Metric:         metricCPUUtilReq,
			ThresholdRatio: 0.80,
		}),
		normalizeRule(config.NotifyRule{
			Name:           "default-mem-util-vs-request",
			Metric:         metricMemUtilReq,
			ThresholdRatio: 0.80,
		}),
	}, true
}

func normalizeRule(r config.NotifyRule) config.NotifyRule {
	r.Name = strings.TrimSpace(r.Name)
	r.Metric = strings.TrimSpace(r.Metric)
	if r.HoldForSeconds <= 0 {
		r.HoldForSeconds = defaultHoldSeconds
	}
	if r.ClearAfterSeconds <= 0 {
		r.ClearAfterSeconds = r.HoldForSeconds
	}
	if r.RepeatSeconds == nil {
		v := defaultRepeatSeconds
		r.RepeatSeconds = &v
	}
	return r
}

func ruleRepeatSeconds(r config.NotifyRule) int {
	if r.RepeatSeconds == nil {
		return defaultRepeatSeconds
	}
	return *r.RepeatSeconds
}

// ResolveNotifyOnResolve returns whether to send resolve events (default true).
func ResolveNotifyOnResolve(r config.NotifyRule) bool {
	if r.NotifyOnResolve == nil {
		return true
	}
	return *r.NotifyOnResolve
}

// FormatBuiltinLog returns the startup log line when builtins are used.
func FormatBuiltinLog() string {
	return BuiltinDefaultsLog
}

// ValidateMetricName reports whether metric is supported.
func ValidateMetricName(m string) error {
	switch strings.TrimSpace(m) {
	case metricCPUUtilReq, metricMemUtilReq, metricCPUUtilLim, metricMemUtilLim,
		metricCPUUtilAbs, metricMemUtilAbs:
		return nil
	default:
		return fmt.Errorf("unsupported metric %q", m)
	}
}
