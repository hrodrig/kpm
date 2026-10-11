package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSampleProfile(t *testing.T) {
	// Load the committed sample from configs/ relative to module root.
	root := findModuleRoot(t)
	path := filepath.Join(root, "configs", "kpm.sample.yml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Store.RetentionDays != 14 {
		t.Fatalf("retention_days: got %d", cfg.Store.RetentionDays)
	}
	if cfg.Sample.IntervalSeconds != 300 {
		t.Fatalf("interval_seconds: got %d", cfg.Sample.IntervalSeconds)
	}
	if cfg.Recommend.TargetUtilization != 0.70 {
		t.Fatalf("target_utilization: got %v", cfg.Recommend.TargetUtilization)
	}
	expanded, err := cfg.ExpandedStorePath()
	if err != nil {
		t.Fatal(err)
	}
	if expanded == "" || expanded[0] == '~' {
		t.Fatalf("store path not expanded: %q", expanded)
	}
}

func TestLoadRejectsBadRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	body := `
store:
  path: /tmp/kpm-test.db
  retention_days: -1
sample:
  interval_seconds: 0
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for retention_days < 1")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpandHome("~/foo/bar.db")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "foo", "bar.db")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got, err = ExpandHome("~")
	if err != nil || got != home {
		t.Fatalf("~ => %q err=%v", got, err)
	}
	got, err = ExpandHome("/abs/path.db")
	if err != nil || got != "/abs/path.db" {
		t.Fatalf("abs => %q err=%v", got, err)
	}
	if _, err := ExpandHome("  "); err == nil {
		t.Fatal("empty path should fail")
	}
}

func TestLoadValidateBranches(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := func(extra string) string {
		return `
store:
  path: ` + filepath.Join(dir, "m.db") + `
  retention_days: 14
sample:
  interval_seconds: 0
` + extra
	}
	if _, err := Load(write("bad-interval.yaml", `
store:
  path: `+filepath.Join(dir, "m.db")+`
  retention_days: 14
sample:
  interval_seconds: -1
`)); err == nil {
		t.Fatal("expected bad interval")
	}
	if _, err := Load(write("bad-pct.yaml", base(`
recommend:
  percentile: 101
`))); err == nil {
		t.Fatal("expected bad percentile")
	}
	if _, err := Load(write("bad-target.yaml", base(`
recommend:
  target_utilization: 1.5
`))); err == nil {
		t.Fatal("expected bad target")
	}
	if _, err := Load(write("bad-baseline.yaml", base(`
tui:
  baseline: bogus
`))); err == nil {
		t.Fatal("expected bad baseline")
	}
	if _, err := Load(write("notify-no-sinks.yaml", base(`
notifications:
  enabled: true
  sinks: []
`))); err == nil {
		t.Fatal("expected enabled notify without sinks to fail")
	}
	cfg, err := Load(write("notify-ok.yaml", base(`
notifications:
  enabled: true
  sinks:
    - type: slack
      webhook_url_env: KPM_SLACK_WEBHOOK
`)))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Notifications.Enabled || len(cfg.Notifications.Sinks) != 1 {
		t.Fatalf("notify cfg: %+v", cfg.Notifications)
	}
	if _, err := Load(write("notify-bad-metric.yaml", base(`
notifications:
  enabled: true
  sinks:
    - type: webhook
      url_env: KPM_WEBHOOK_URL
      body: discord
  rules:
    - name: x
      metric: not.a.metric
      threshold_ratio: 0.9
`))); err == nil {
		t.Fatal("bad metric")
	}
	if _, err := Load(write("notify-bad-sink.yaml", base(`
notifications:
  enabled: true
  sinks:
    - type: pagerduty
`))); err == nil {
		t.Fatal("bad sink type")
	}
	if _, err := Load(write("notify-smtp.yaml", base(`
notifications:
  enabled: true
  sinks:
    - type: smtp
      host_env: KPM_SMTP_HOST
      port_env: KPM_SMTP_PORT
      from_env: KPM_SMTP_FROM
      to_env: KPM_SMTP_TO
`))); err != nil {
		t.Fatal(err)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
