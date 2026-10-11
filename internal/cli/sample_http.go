package cli

import (
	"context"
	"fmt"
	"io"

	"k8s.io/client-go/kubernetes"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
	"github.com/hrodrig/kpm/internal/sample"
	"github.com/hrodrig/kpm/internal/store"
)

// startOpsHTTP starts the daemon ops mux when mcp.enabled. Returns counters
// for tick instrumentation (nil when HTTP is off).
func startOpsHTTP(
	ctx context.Context,
	cfg *config.Config,
	st *store.Store,
	cs kubernetes.Interface,
	errW io.Writer,
) (*httpserver.Counters, error) {
	if !cfg.MCP.Enabled {
		return nil, nil
	}
	ready := func(rctx context.Context) error {
		if _, err := st.Count(rctx); err != nil {
			return fmt.Errorf("store: %w", err)
		}
		if cs != nil {
			if _, err := cs.Discovery().ServerVersion(); err != nil {
				return fmt.Errorf("kube: %w", err)
			}
		}
		return nil
	}
	srv := httpserver.New(cfg.MCP.Listen, ready, &httpserver.Counters{})
	_, _ = fmt.Fprintf(errW, "httpserver: listening on %s (/healthz /readyz /metrics)\n", srv.Addr())
	go func() {
		if err := srv.ListenAndServe(ctx); err != nil {
			_, _ = fmt.Fprintf(errW, "httpserver: %v\n", err)
		}
	}()
	return srv.Metrics(), nil
}

// withProcessMetrics increments daemon ops counters around each tick.
func withProcessMetrics(base sample.TickFunc, m *httpserver.Counters) sample.TickFunc {
	if m == nil {
		return base
	}
	return func(res sample.Result) {
		m.SampleTicks.Add(1)
		if res.StoreWrite > 0 {
			m.ObserveSampleWrite(res.StoreWrite)
		}
		base(res)
	}
}
