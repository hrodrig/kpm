package mcp

import (
	"fmt"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
)

// PathSSE is the legacy SSE MCP endpoint (SPEC §7.5 client examples).
const PathSSE = "/sse"

// PathMCP is the streamable HTTP MCP endpoint.
const PathMCP = "/mcp"

// ResolveBearerToken reads token_env. Empty token_env means no auth (loopback only).
// If token_env is set, the env value must be non-empty (fail closed).
func ResolveBearerToken(tokenEnv string) (string, error) {
	return httpserver.ResolveTokenEnv(tokenEnv)
}

// BearerAuth wraps next and requires Authorization: Bearer <token> when token
// is non-empty. Ops probes must not use this wrapper.
func BearerAuth(token string, next http.Handler) http.Handler {
	return httpserver.BearerAuth(token, next)
}

// MountHTTP registers SSE (/sse) and streamable HTTP (/mcp) on mux, optionally
// behind bearer auth. Uses the same tool surface as stdio.
func MountHTTP(mux *http.ServeMux, d *Deps, bearerToken string) {
	srv := NewServer(d)
	get := func(*http.Request) *mcpsdk.Server { return srv }
	sse := mcpsdk.NewSSEHandler(get, nil)
	stream := mcpsdk.NewStreamableHTTPHandler(get, nil)
	mux.Handle(PathSSE, BearerAuth(bearerToken, sse))
	mux.Handle(PathSSE+"/", BearerAuth(bearerToken, sse))
	mux.Handle(PathMCP, BearerAuth(bearerToken, stream))
	mux.Handle(PathMCP+"/", BearerAuth(bearerToken, stream))
}

// AttachDaemonHTTP mounts MCP on an ops Server mux after resolving bearer token
// and installing gghstats-style access control (allowlist + rate limit).
func AttachDaemonHTTP(ops *httpserver.Server, d *Deps, mcpCfg config.MCP) error {
	tok, err := ResolveBearerToken(mcpCfg.TokenEnv)
	if err != nil {
		return err
	}
	if tok == "" && !httpserver.IsLoopbackListen(ops.Addr()) {
		return fmt.Errorf("mcp HTTP: bearer token required for non-loopback listen %q", ops.Addr())
	}
	ac := BuildAccessControl(mcpCfg, tok)
	ops.SetAccessControl(ac)
	rlOn, alOn := ac.Active()
	httpserver.WarnTrustedProxiesIfNeeded(ac.Trusted, rlOn, alOn)
	MountHTTP(ops.Mux(), d, tok)
	return nil
}

// BuildAccessControl builds allowlist + rate-limit middleware from MCP config.
func BuildAccessControl(mcpCfg config.MCP, bearer string) httpserver.AccessControl {
	trusted := httpserver.ParseTrustedProxies(mcpCfg.TrustedProxies...)
	ac := httpserver.AccessControl{Trusted: trusted}
	ac.Allowlist = httpserver.NewAllowlist(mcpCfg.AllowlistCIDRs, bearer, trusted)
	if mcpCfg.MCPRateLimitEnabled() {
		req, period, burst := mcpCfg.MCPRateLimitResolved()
		ac.RateLimit = httpserver.NewRateLimiter(httpserver.RateLimitConfig{
			Enabled:  true,
			Requests: req,
			Period:   period,
			Burst:    burst,
		}, trusted)
	}
	return ac
}
