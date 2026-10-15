package sample

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/client-go/kubernetes"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

// TickFunc is called after each successful sample tick (tests / CLI logging).
type TickFunc func(Result)

// SampleRetryBudget is how long the daemon keeps retrying transient RunOnce
// errors before failing closed. Not configurable in YAML (documented constant).
const SampleRetryBudget = 2 * time.Minute

// sampleRetryBudget is overridable in tests.
var sampleRetryBudget = SampleRetryBudget

// FailHook is invoked on each transient sample failure (tests / ops counters).
type FailHook func(consecutive int, err error)

// runOnceFn is the per-tick sampler (overridable in tests).
var runOnceFn = RunOnce

// onSampleFailFn is optional (overridable in tests); default logs a warning.
var onSampleFailFn FailHook = LogSampleFail

// LogSampleFail is the default FailHook (structured warning).
func LogSampleFail(consecutive int, err error) {
	slog.Warn("sample tick failed; retrying",
		"consecutive", consecutive,
		"err", err.Error(),
	)
}

// SetFailHook replaces the transient-failure hook and returns the previous one.
// Pass nil to restore LogSampleFail.
func SetFailHook(h FailHook) FailHook {
	prev := onSampleFailFn
	if h == nil {
		onSampleFailFn = LogSampleFail
	} else {
		onSampleFailFn = h
	}
	return prev
}

// RunLoop samples forever until ctx is cancelled. intervalSeconds must be > 0.
// Transient cluster/API errors are retried with exponential backoff until
// SampleRetryBudget elapses; persistent failures fail closed.
// On cancel: returns nil (clean shutdown).
func RunLoop(
	ctx context.Context,
	cfg *config.Config,
	st *store.Store,
	pods kubernetes.Interface,
	metrics metricsclientset.Interface,
	onTick TickFunc,
) error {
	if cfg.Sample.IntervalSeconds < 1 {
		return fmt.Errorf("sample.interval_seconds must be >= 1 for daemon loop")
	}
	interval := time.Duration(cfg.Sample.IntervalSeconds) * time.Second
	fails := 0
	var failSince time.Time

	for {
		res, err := runOnceFn(ctx, cfg, st, pods, metrics)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if fails == 0 {
				failSince = time.Now()
			}
			fails++
			if onSampleFailFn != nil {
				onSampleFailFn(fails, err)
			}
			if time.Since(failSince) >= sampleRetryBudget {
				return fmt.Errorf("sample loop: retry budget %s exceeded after %d failures: %w",
					sampleRetryBudget, fails, err)
			}
			backoff := retryBackoff(fails, interval)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		fails = 0
		failSince = time.Time{}
		if onTick != nil {
			onTick(res)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func retryBackoff(fails int, interval time.Duration) time.Duration {
	// 1s, 2s, 4s, 8s, 16s, 32s… capped at sample interval.
	shift := fails - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 5 {
		shift = 5
	}
	backoff := time.Duration(1<<uint(shift)) * time.Second
	if backoff > interval {
		return interval
	}
	return backoff
}
