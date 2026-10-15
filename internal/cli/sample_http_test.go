package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
	"github.com/hrodrig/kpm/internal/sample"
	"github.com/hrodrig/kpm/internal/store"
)

func TestStartOpsHTTPDisabled(t *testing.T) {
	m, err := startOpsHTTP(context.Background(), &config.Config{}, nil, nil, &bytes.Buffer{})
	if err != nil || m != nil {
		t.Fatalf("got metrics=%v err=%v", m, err)
	}
}

func TestStartOpsHTTPListen(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{}
	cfg.MCP.Enabled = true
	cfg.MCP.Listen = "127.0.0.1:0"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errBuf := &bytes.Buffer{}
	m, err := startOpsHTTP(ctx, cfg, st, nil, errBuf)
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("expected counters")
	}
	time.Sleep(80 * time.Millisecond)
	log := errBuf.String()
	if !bytes.Contains(errBuf.Bytes(), []byte("httpserver: listening")) ||
		!bytes.Contains(errBuf.Bytes(), []byte("/sse")) {
		t.Fatalf("log: %q", log)
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestNewOpsHTTPServerOnly(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := &config.Config{}
	cfg.Server.Enabled = true
	cfg.Server.Listen = "127.0.0.1:0"
	srv, err := newOpsHTTP(cfg, st, nil)
	if err != nil || srv == nil {
		t.Fatalf("srv=%v err=%v", srv, err)
	}
	rlOn, _ := srv.Access().Active()
	if !rlOn {
		t.Fatal("server-only must attach rate-limit from mcp.* defaults")
	}
}

func TestNewOpsHTTPServerOnlyAllowlist(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := &config.Config{}
	cfg.Server.Enabled = true
	cfg.Server.Listen = "127.0.0.1:0"
	cfg.MCP.AllowlistCIDRs = []string{"10.0.0.0/8"}
	falseV := false
	cfg.MCP.RateLimit.Enabled = &falseV
	srv, err := newOpsHTTP(cfg, st, nil)
	if err != nil || srv == nil {
		t.Fatalf("srv=%v err=%v", srv, err)
	}
	_, alOn := srv.Access().Active()
	if !alOn {
		t.Fatal("server-only must honor mcp.allowlist_cidrs")
	}
}

func TestMergeAllowedHosts(t *testing.T) {
	got := mergeAllowedHosts([]string{"a.example", "b.example"}, []string{"b.example", "c.example", ""})
	if len(got) != 3 || got[0] != "a.example" || got[2] != "c.example" {
		t.Fatalf("got %#v", got)
	}
}

func TestWithProcessMetrics(t *testing.T) {
	c := &httpserver.Counters{}
	called := false
	fn := withProcessMetrics(func(sample.Result) { called = true }, c)
	fn(sample.Result{StoreWrite: 5 * time.Millisecond})
	if !called {
		t.Fatal("base not called")
	}
	if c.SampleTicks.Load() != 1 || c.SampleWriteCount.Load() != 1 {
		t.Fatalf("counters: ticks=%d writes=%d", c.SampleTicks.Load(), c.SampleWriteCount.Load())
	}
	if withProcessMetrics(func(sample.Result) {}, nil) == nil {
		t.Fatal("nil metrics should return base")
	}
}
