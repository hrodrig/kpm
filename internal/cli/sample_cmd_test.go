package cli

import (
	"bytes"
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

func TestSampleHelpListsCommand(t *testing.T) {
	printSampleConfig = false
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"sample", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "interval_seconds") {
		t.Fatalf("help missing interval mention: %q", buf.String())
	}
}
