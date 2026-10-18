package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/store"
)

func TestBearerAuth(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := mcp.BearerAuth("secret", ok)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/sse", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("with auth: %d", rr.Code)
	}

	open := mcp.BearerAuth("", ok)
	rr = httptest.NewRecorder()
	open.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("empty token: %d", rr.Code)
	}
}

func TestResolveBearerToken(t *testing.T) {
	t.Setenv("KPM_MCP_TOKEN_TEST", "")
	if _, err := mcp.ResolveBearerToken("KPM_MCP_TOKEN_TEST"); err == nil {
		t.Fatal("empty env")
	}
	t.Setenv("KPM_MCP_TOKEN_TEST", "tok")
	got, err := mcp.ResolveBearerToken("KPM_MCP_TOKEN_TEST")
	if err != nil || got != "tok" {
		t.Fatalf("got %q err=%v", got, err)
	}
	got, err = mcp.ResolveBearerToken("")
	if err != nil || got != "" {
		t.Fatalf("unset: %q %v", got, err)
	}
}

func TestMountHTTPOpsUnauthenticated(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ops := httpserver.New("127.0.0.1:9740", nil, nil)
	d := &mcp.Deps{Store: st, Config: &config.Config{}, Version: "t"}
	off := false
	mcpCfg := config.MCP{Enabled: true, Listen: "127.0.0.1:9740", RateLimit: config.MCPRateLimit{Enabled: &off}}
	if err := mcp.AttachDaemonHTTP(ops, d, mcpCfg); err != nil {
		t.Fatal(err)
	}
	// Ops stay open without bearer.
	rr := httptest.NewRecorder()
	ops.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz: %d", rr.Code)
	}

	// MCP with token required when we attach with token_env.
	t.Setenv("KPM_MCP_HTTP_TEST", "s3cret")
	ops2 := httpserver.New("127.0.0.1:9740", nil, nil)
	mcpCfg2 := config.MCP{
		Enabled: true, Listen: "127.0.0.1:9740", TokenEnv: "KPM_MCP_HTTP_TEST",
		RateLimit: config.MCPRateLimit{Enabled: &off},
	}
	if err := mcp.AttachDaemonHTTP(ops2, d, mcpCfg2); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	reqSSE := httptest.NewRequest(http.MethodGet, mcp.PathSSE, nil)
	reqSSE.Host = "127.0.0.1:9740"
	ops2.Handler().ServeHTTP(rr, reqSSE)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("sse no auth: %d", rr.Code)
	}
	// Do not invoke the real SSE handler with a valid bearer: go-sdk waits on the
	// event stream and would hang httptest. Bearer success is covered in TestBearerAuth.

	ops3 := httpserver.New("0.0.0.0:9740", nil, nil)
	mcpCfg3 := config.MCP{Enabled: true, Listen: "0.0.0.0:9740", RateLimit: config.MCPRateLimit{Enabled: &off}}
	if err := mcp.AttachDaemonHTTP(ops3, d, mcpCfg3); err == nil {
		t.Fatal("expected non-loopback without token to fail")
	}
}

func TestBuildAccessControlRejectsBadTrustedProxy(t *testing.T) {
	t.Parallel()
	_, err := mcp.BuildAccessControl(config.MCP{
		TrustedProxies: []string{"10.0.0.0/8", "10.0.0.0/33"},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "10.0.0.0/33") {
		t.Fatalf("unparseable trusted_proxies must refuse to start: %v", err)
	}
}
