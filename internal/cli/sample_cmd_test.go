package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSampleMissingConfig(t *testing.T) {
	printSampleConfig = false
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"sample", "--config", filepath.Join(t.TempDir(), "missing.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestSampleRejectsDaemonInterval(t *testing.T) {
	printSampleConfig = false
	dir := t.TempDir()
	path := filepath.Join(dir, "kpm.yaml")
	body := `
cluster:
  name: t
kube:
  context: ""
client:
  id: test-client
store:
  path: ` + filepath.Join(dir, "m.db") + `
  retention_days: 14
sample:
  interval_seconds: 60
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd()
	errBuf := &bytes.Buffer{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"sample", "--config", path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected daemon interval rejection")
	}
	if !strings.Contains(err.Error(), "roadmap #4") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSampleHelpListsCommand(t *testing.T) {
	printSampleConfig = false
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"sample", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "SQLite") {
		t.Fatalf("help missing SQLite mention: %q", buf.String())
	}
}
