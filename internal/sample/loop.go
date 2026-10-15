package sample

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

// TickFunc is called after each successful sample tick (tests / CLI logging).
type TickFunc func(Result)

// MaxConsecutiveSampleFailures is how many transient RunOnce errors the daemon
// tolerates before failing closed. Not configurable in YAML (documented constant).
const MaxConsecutiveSampleFailures = 5

// runOnceFn is the per-tick sampler (overridable in tests).
var runOnceFn = RunOnce

// RunLoop samples forever until ctx is cancelled. intervalSeconds must be > 0.
// Transient cluster/API errors are retried with linear backoff up to
// MaxConsecutiveSampleFailures; persistent failures fail closed.
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

	for {
		res, err := runOnceFn(ctx, cfg, st, pods, metrics)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fails++
			if fails >= MaxConsecutiveSampleFailures {
				return fmt.Errorf("sample loop: %d consecutive failures: %w", fails, err)
			}
			backoff := time.Duration(fails) * time.Second
			if backoff > interval {
				backoff = interval
			}
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
