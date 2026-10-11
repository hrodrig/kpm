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

// RunLoop samples forever until ctx is cancelled. intervalSeconds must be > 0.
// On cancel: returns nil (clean shutdown). Cluster/API errors fail closed.
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

	for {
		res, err := RunOnce(ctx, cfg, st, pods, metrics)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
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
