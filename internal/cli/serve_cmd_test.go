package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func TestServeCommandRegistered(t *testing.T) {
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"serve", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Analyze-local") || !strings.Contains(out, "mcp.enabled") {
		t.Fatalf("help: %q", out)
	}
}

func TestServeRequiresMCPEnabled(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "s.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	cfgPath := filepath.Join(dir, "kpm.yaml")
	body := "store:\n  path: " + db + "\n  retention_days: 7\nsample:\n  interval_seconds: 0\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	errBuf := &bytes.Buffer{}
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"serve", "--config", cfgPath})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "mcp.enabled") {
		t.Fatalf("want mcp.enabled error, got %v", err)
	}
}

func TestNewOpsHTTPReadyStoreOnlyAndNoWrites(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	before, err := st.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.MCP.Enabled = true
	cfg.MCP.Listen = "127.0.0.1:0"
	srv, err := newOpsHTTP(cfg, st, nil)
	if err != nil || srv == nil {
		t.Fatalf("newOpsHTTP: srv=%v err=%v", srv, err)
	}

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("readyz %d body=%q", rr.Code, rr.Body.String())
	}

	after, err := st.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("store writes during ops HTTP build: before=%d after=%d", before, after)
	}
}

func TestServeListenNoSampleWrites(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "s.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	cfgPath := filepath.Join(dir, "kpm.yaml")
	body := "" +
		"store:\n  path: " + db + "\n  retention_days: 7\n" +
		"sample:\n  interval_seconds: 0\n" +
		"mcp:\n  enabled: true\n  listen: \"127.0.0.1:0\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"serve", "--config", cfgPath})

	errCh := make(chan error, 1)
	go func() { errCh <- cmd.ExecuteContext(ctx) }()

	// Let ListenAndServe bind; no concurrent Buffer.String (race under -race).
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve exit: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not stop")
	}
	if !strings.Contains(out.String(), "kpm serve:") {
		t.Fatalf("no serve banner; out=%q err=%q", out.String(), errBuf.String())
	}

	st2, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	after, err := st2.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("unexpected sample writes: before=%d after=%d", before, after)
	}
}
