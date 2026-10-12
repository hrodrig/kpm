package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/sample"
	"github.com/hrodrig/kpm/internal/server"
	"github.com/hrodrig/kpm/internal/store"
)

// newOpsHTTP builds the ops mux when mcp.enabled and/or server.enabled.
// /readyz requires an open store; kube/live is optional.
func newOpsHTTP(
	cfg *config.Config,
	st *store.Store,
	liveFn func(context.Context) (server.LiveOut, error),
) (*httpserver.Server, error) {
	if cfg == nil || (!cfg.MCP.Enabled && !cfg.Server.Enabled) {
		return nil, nil
	}
	ready := func(rctx context.Context) error {
		if _, err := st.Count(rctx); err != nil {
			return fmt.Errorf("store: %w", err)
		}
		return nil
	}
	addr := cfg.HTTPListen()
	srv := httpserver.New(addr, ready, &httpserver.Counters{})
	mcpTok, apiTok, err := resolveHTTPTokens(cfg)
	if err != nil {
		return nil, err
	}
	if err := requireHTTPTokens(cfg, addr, mcpTok, apiTok); err != nil {
		return nil, err
	}
	mountHTTPFacades(srv, cfg, st, liveFn, mcpTok, apiTok)
	return srv, nil
}

func resolveHTTPTokens(cfg *config.Config) (mcpTok, apiTok string, err error) {
	mcpTok, err = mcp.ResolveBearerToken(cfg.MCP.TokenEnv)
	if err != nil {
		return "", "", err
	}
	apiTok, err = server.ResolveAPIToken(cfg)
	if err != nil {
		return "", "", err
	}
	return mcpTok, apiTok, nil
}

func requireHTTPTokens(cfg *config.Config, addr, mcpTok, apiTok string) error {
	if cfg.MCP.Enabled && mcpTok == "" && !httpserver.IsLoopbackListen(addr) {
		return fmt.Errorf("mcp HTTP: bearer token required for non-loopback listen %q", addr)
	}
	if cfg.Server.Enabled && cfg.ServerAuthRequired(addr) && apiTok == "" {
		return fmt.Errorf("server HTTP: bearer token required for listen %q", addr)
	}
	return nil
}

func mountHTTPFacades(
	srv *httpserver.Server,
	cfg *config.Config,
	st *store.Store,
	liveFn func(context.Context) (server.LiveOut, error),
	mcpTok, apiTok string,
) {
	if cfg.MCP.Enabled {
		ac := mcp.BuildAccessControl(cfg.MCP, mcpTok)
		srv.SetAccessControl(ac)
		rlOn, alOn := ac.Active()
		httpserver.WarnTrustedProxiesIfNeeded(ac.Trusted, rlOn, alOn)
		mcp.MountHTTP(srv.Mux(), &mcp.Deps{Store: st, Config: cfg, Version: Version}, mcpTok)
	}
	if cfg.Server.Enabled {
		server.MountAll(srv.Mux(), &server.Deps{
			Store:   st,
			Config:  cfg,
			Version: Version,
			Live:    liveFn,
		}, apiTok)
	}
}

// startOpsHTTP starts the daemon ops mux when mcp/server enabled.
// Returns counters for tick instrumentation (nil when HTTP is off).
func startOpsHTTP(
	ctx context.Context,
	cfg *config.Config,
	st *store.Store,
	liveFn func(context.Context) (server.LiveOut, error),
	errW io.Writer,
) (*httpserver.Counters, error) {
	srv, err := newOpsHTTP(cfg, st, liveFn)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, nil
	}
	logOpsListen(errW, cfg, srv.Addr())
	go func() {
		if err := srv.ListenAndServe(ctx); err != nil {
			_, _ = fmt.Fprintf(errW, "httpserver: %v\n", err)
		}
	}()
	return srv.Metrics(), nil
}

func logOpsListen(errW io.Writer, cfg *config.Config, addr string) {
	_, _ = fmt.Fprintf(errW, "httpserver: listening on %s (/healthz /readyz /metrics", addr)
	if cfg.MCP.Enabled {
		_, _ = fmt.Fprintf(errW, "; MCP %s + %s", mcp.PathSSE, mcp.PathMCP)
	}
	if cfg.Server.Enabled {
		_, _ = fmt.Fprintf(errW, "; JSON %s/*; UI /", server.APIPrefix)
	}
	_, _ = fmt.Fprint(errW, ")\n")
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
