package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hrodrig/kpm/internal/live"
)

func TestCapacityHelp(t *testing.T) {
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"capacity", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "interval") && !strings.Contains(out, "SQLite") && !strings.Contains(out, "Live") {
		// help should mention live / no store
		if !strings.Contains(out, "capacity") {
			t.Fatalf("help: %q", out)
		}
	}
	if !strings.Contains(out, "Live") && !strings.Contains(out, "live") {
		t.Fatalf("expected live wording: %q", out)
	}
}

func TestCapacityMissingConfig(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"capacity", "--config", filepath.Join(t.TempDir(), "missing.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected config error")
	}
}

func TestCapacityRootListsCommand(t *testing.T) {
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "capacity") {
		t.Fatalf("root help missing capacity: %q", buf.String())
	}
}

func TestCapacitySortFlagDefault(t *testing.T) {
	cmd := NewRootCmd()
	c, _, err := cmd.Find([]string{"capacity"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Flags().Lookup("sort") == nil {
		t.Fatal("missing --sort")
	}
}

func TestWriteCapacityTable(t *testing.T) {
	var out, errOut bytes.Buffer
	snap := live.Snapshot{Containers: []live.ContainerRow{
		{
			Namespace: "app", Pod: "hot", Container: "c", Node: "n1",
			HasUtil: true, CPUUtilMcores: 80, CPURequestMcores: 100,
			MemUtilBytes: 10 << 20, MemRequestBytes: 64 << 20,
		},
		{
			Namespace: "app", Pod: "cool", Container: "c", Node: "n2",
			HasUtil: true, CPUUtilMcores: 10, CPURequestMcores: 100,
			MemUtilBytes: 4 << 20, MemRequestBytes: 64 << 20,
		},
	}}
	if err := writeCapacityTable(&out, &errOut, snap, "", "cpu.ratio"); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "hot") || !strings.Contains(errOut.String(), "2 pods") {
		t.Fatalf("out=%q err=%q", s, errOut.String())
	}
	// hottest first
	if idxHot, idxCool := strings.Index(s, "hot"), strings.Index(s, "cool"); idxHot < 0 || idxCool < 0 || idxHot > idxCool {
		t.Fatalf("sort order: %q", s)
	}
}

func TestWriteCapacityTableNameSort(t *testing.T) {
	var out, errOut bytes.Buffer
	snap := live.Snapshot{Containers: []live.ContainerRow{
		{Namespace: "b", Pod: "z", Container: "c", CPURequestMcores: 1},
		{Namespace: "a", Pod: "y", Container: "c", CPURequestMcores: 1},
	}}
	if err := writeCapacityTable(&out, &errOut, snap, "name", ""); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	idxA := strings.Index(s, "a          y")
	idxB := strings.Index(s, "b          z")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Fatalf("name sort: %q", s)
	}
}
