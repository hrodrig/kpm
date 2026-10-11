package cli

import (
	"context"
	"fmt"
	"io"

	"k8s.io/client-go/kubernetes"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/sample"
	"github.com/hrodrig/kpm/internal/store"
)

// newOpsHTTP builds the ops mux + MCP HTTP when mcp.enabled.
// /readyz requires an open store; kube (cs) is optional and not checked.
// Returns nil when MCP is disabled.
func newOpsHTTP(
	cfg *config.Config,
	st *store.Store,
	cs kubernetes.Interface,
) (*httpserver.Server, error) {
	if !cfg.MCP.Enabled {
		return nil, nil
	}
	_ = cs // reserved for optional live enrichment (#19); store-only ready today
	ready := func(rctx context.Context) error {
		if _, err := st.Count(rctx); err != nil {
			return fmt.Errorf("store: %w", err)
		}
		return nil
	}
	srv := httpserver.New(cfg.MCP.Listen, ready, &httpserver.Counters{})
	deps := &mcp.Deps{Store: st, Config: cfg, Version: Version}
	if err := mcp.AttachDaemonHTTP(srv, deps, cfg.MCP); err != nil {
		return nil, err
	}
	return srv, nil
}

// startOpsHTTP starts the daemon ops mux + MCP HTTP when mcp.enabled.
// Returns counters for tick instrumentation (nil when HTTP is off).
func startOpsHTTP(
	ctx context.Context,
	cfg *config.Config,
	st *store.Store,
	cs kubernetes.Interface,
	errW io.Writer,
) (*httpserver.Counters, error) {
	srv, err := newOpsHTTP(cfg, st, cs)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, nil
	}
	_, _ = fmt.Fprintf(errW,
		"httpserver: listening on %s (/healthz /readyz /metrics; MCP %s + %s)\n",
		srv.Addr(), mcp.PathSSE, mcp.PathMCP)
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
