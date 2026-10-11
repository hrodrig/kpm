package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestTUIHelp(t *testing.T) {
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tui", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "heatmap") && !strings.Contains(buf.String(), "bubbletea") {
		t.Fatalf("help: %q", buf.String())
	}
}

func TestCapacityTUIFlag(t *testing.T) {
	cmd := NewRootCmd()
	c, _, err := cmd.Find([]string{"capacity"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Flags().Lookup("tui") == nil {
		t.Fatal("missing --tui")
	}
}

func TestTUIMissingConfig(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"tui", "--config", filepath.Join(t.TempDir(), "no.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}
