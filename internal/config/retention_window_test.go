package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureSlog redirects the default logger into a buffer for the duration of fn
// and returns everything written. Config warns through slog (same channel as
// httpserver/trusted.go), so asserting the warning means capturing the handler.
func captureSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	fn()
	return buf.String()
}

func writeConfig(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const thinRetentionMsg = "retention_days is shorter than recommend.window_days"

// #24: retention_days < recommend.window_days must warn and still load.
// Fail-closed would contradict SPECIFICATIONS.md §2.2, which states the
// relation as guidance ("prefer"), not as a requirement.
func TestLoadWarnsWhenRetentionThinnerThanWindow(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, "thin.yaml", `
store:
  path: `+filepath.Join(dir, "m.db")+`
  retention_days: 3
sample:
  interval_seconds: 0
recommend:
  window_days: 7
`)

	var cfg *Config
	var err error
	out := captureSlog(t, func() { cfg, err = Load(path) })

	if err != nil {
		t.Fatalf("thin retention must load with a warning, not fail: %v", err)
	}
	if cfg.Store.RetentionDays != 3 {
		t.Fatalf("retention_days: got %d", cfg.Store.RetentionDays)
	}
	if cfg.Recommend.WindowDays != 7 {
		t.Fatalf("window_days: got %d", cfg.Recommend.WindowDays)
	}
	if !strings.Contains(out, thinRetentionMsg) {
		t.Fatalf("expected thin-retention warning, got: %q", out)
	}
}

// Boundary: retention exactly equal to the window covers it, so no warning.
func TestLoadNoWarnWhenRetentionEqualsWindow(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, "equal.yaml", `
store:
  path: `+filepath.Join(dir, "m.db")+`
  retention_days: 7
sample:
  interval_seconds: 0
recommend:
  window_days: 7
`)

	var err error
	out := captureSlog(t, func() { _, err = Load(path) })

	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, thinRetentionMsg) {
		t.Fatalf("equal retention must not warn, got: %q", out)
	}
}

// Defaults are safe: sample retention 14 against the default window of 7 must
// stay silent, otherwise every default config would emit noise.
func TestLoadNoWarnWithDefaultRetentionAndWindow(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, "defaults.yaml", `
store:
  path: `+filepath.Join(dir, "m.db")+`
sample:
  interval_seconds: 0
`)

	var err error
	out := captureSlog(t, func() { _, err = Load(path) })

	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, thinRetentionMsg) {
		t.Fatalf("default config must not warn, got: %q", out)
	}
}
