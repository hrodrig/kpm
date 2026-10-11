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

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/sample"
)

func TestNotifyTestSends(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		got = buf.String()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	t.Setenv("KPM_TEST_NOTIFY_SLACK", srv.URL)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kpm.yaml")
	body := `
store:
  path: ` + filepath.Join(dir, "m.db") + `
  retention_days: 14
sample:
  interval_seconds: 0
notifications:
  enabled: true
  sinks:
    - type: slack
      webhook_url_env: KPM_TEST_NOTIFY_SLACK
`
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"notify-test", "--config", cfgPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "notify-test: sent") {
		t.Fatalf("out=%q", out.String())
	}
	if !strings.Contains(got, "kpm ALERT") {
		t.Fatalf("payload=%q", got)
	}
}

func TestWrapDaemonTickDisabled(t *testing.T) {
	cfg := &config.Config{}
	called := false
	logTick := func(sample.Result) { called = true }
	fn, err := wrapDaemonTick(context.Background(), cfg, "c", "k", logTick, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	fn(sample.Result{})
	if !called {
		t.Fatal("logTick not called")
	}
}

func TestWrapDaemonTickEnabledFailClosed(t *testing.T) {
	cfg := &config.Config{}
	cfg.Notifications.Enabled = true
	cfg.Notifications.Sinks = []config.NotifySink{{
		Type: "slack", WebhookURLEnv: "KPM_MISSING_WRAP_SLACK",
	}}
	_, err := wrapDaemonTick(context.Background(), cfg, "c", "k", func(sample.Result) {}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected fail-closed")
	}
}

func TestWrapDaemonTickEnabledOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	t.Setenv("KPM_WRAP_SLACK_OK", srv.URL)
	cfg := &config.Config{}
	cfg.Notifications.Enabled = true
	cfg.Notifications.Sinks = []config.NotifySink{{
		Type: "slack", WebhookURLEnv: "KPM_WRAP_SLACK_OK",
	}}
	errBuf := &bytes.Buffer{}
	fn, err := wrapDaemonTick(context.Background(), cfg, "c", "k", func(sample.Result) {}, errBuf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errBuf.String(), "built-in defaults") {
		t.Fatalf("want builtin log: %q", errBuf.String())
	}
	fn(sample.Result{}) // no breach — still OK
}
