package cli

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/server"
)

// liveFnFromClients builds a /api/v1/live capture func from typed clients.
// Returns nil when either client is nil.
func liveFnFromClients(
	cs kubernetes.Interface,
	mcs metricsclientset.Interface,
	scope config.Scope,
) func(context.Context) (server.LiveOut, error) {
	if cs == nil || mcs == nil {
		return nil
	}
	return func(ctx context.Context) (server.LiveOut, error) {
		snap, err := live.CaptureClients(ctx, cs, mcs, live.Options{Scope: scope})
		if err != nil {
			return server.LiveOut{}, err
		}
		out := server.LiveOut{
			CapturedAt: snap.CapturedAt.UTC().Format(time.RFC3339),
			Rows:       make([]server.LiveRow, 0, len(snap.Containers)),
		}
		for _, c := range snap.Containers {
			out.Rows = append(out.Rows, server.LiveRow{
				Namespace:        c.Namespace,
				Pod:              c.Pod,
				Container:        c.Container,
				OwnerKind:        c.OwnerKind,
				OwnerName:        c.OwnerName,
				CPUUtilMcores:    c.CPUUtilMcores,
				CPURequestMcores: c.CPURequestMcores,
				CPULimitMcores:   c.CPULimitMcores,
				MemUtilBytes:     c.MemUtilBytes,
				MemRequestBytes:  c.MemRequestBytes,
				MemLimitBytes:    c.MemLimitBytes,
				HasUtil:          c.HasUtil,
			})
		}
		out.Count = len(out.Rows)
		return out, nil
	}
}
