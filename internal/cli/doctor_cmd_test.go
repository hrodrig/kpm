package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/store"
)

// RunE must surface report.Broken() as an exit error after the report is
// printed. Dropping that return makes a broken doctor look successful.
func TestDoctorRunEExitFollowsBroken(t *testing.T) {
	t.Run("missing store is exit 1 and still printed", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := writeMiniConfig(t, dir, filepath.Join(dir, "absent.db"))
		out := &bytes.Buffer{}
		cmd := NewRootCmd()
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"doctor", "--config", cfgPath})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("broken doctor must not exit 0")
		}
		if got := exitcode.Of(err); got != exitcode.Failure {
			t.Fatalf("exit code: got %d want %d (%v)", got, exitcode.Failure, err)
		}
		if !strings.Contains(out.String(), "store.path") || !strings.Contains(out.String(), "fail") {
			t.Fatalf("report must be printed before the exit error: %q", out.String())
		}
	})

	t.Run("warnings alone stay exit 0", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "empty.db")
		st, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
		cfgPath := writeMiniConfig(t, dir, dbPath)
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"doctor", "--config", cfgPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("warn-only doctor must exit 0: %v", err)
		}
	})

	t.Run("cluster-only failure is exit 2", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "empty.db")
		st, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
		cfgPath := writeMiniConfig(t, dir, dbPath)
		out := &bytes.Buffer{}
		cmd := NewRootCmd()
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{
			"doctor", "--config", cfgPath, "--live",
			"--kubeconfig", filepath.Join(dir, "missing-kubeconfig"),
		})

		err = cmd.Execute()
		if err == nil {
			t.Fatal("live probe failure must not exit 0")
		}
		if got := exitcode.Of(err); got != exitcode.Cluster {
			t.Fatalf("exit code: got %d want %d (%v)", got, exitcode.Cluster, err)
		}
		if !strings.Contains(out.String(), "cluster.metrics_api") {
			t.Fatalf("cluster failure must be in the printed report: %q", out.String())
		}
	})
}
