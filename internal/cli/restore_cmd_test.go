package cli

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/hrodrig/kpm/internal/store"
)

// seedStoreFile writes one sample per pod, one minute apart, from base.
func seedStoreFile(t *testing.T, path string, base time.Time, pods ...string) {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	recs := make([]store.Record, 0, len(pods))
	for i, p := range pods {
		recs = append(recs, store.Record{
			SampledAt:     base.Add(time.Duration(i) * time.Minute),
			ClientID:      "test",
			ClusterName:   "t",
			Namespace:     "app",
			Pod:           p,
			Container:     "c",
			CPUUtilMcores: 100,
		})
	}
	if err := st.InsertRecords(t.Context(), recs); err != nil {
		t.Fatal(err)
	}
}

func backupOf(t *testing.T, storePath, backupPath string) {
	t.Helper()
	st, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Backup(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
}

func countSamples(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM samples").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRestoreCommandReplacesStore(t *testing.T) {
	printSampleConfig = false
	dir := t.TempDir()
	base := time.Now().UTC().Truncate(time.Second)

	srcPath := filepath.Join(dir, "src.db")
	seedStoreFile(t, srcPath, base, "a", "b", "c")
	bak := filepath.Join(dir, "bak.db")
	backupOf(t, srcPath, bak)

	dstPath := filepath.Join(dir, "home", "metrics.db")
	cfgPath := writeMiniConfig(t, dir, dstPath)

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetArgs([]string{"restore", "--config", cfgPath, bak})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !strings.Contains(out.String(), dstPath) {
		t.Fatalf("restore out: %q", out.String())
	}
	if n := countSamples(t, dstPath); n != 3 {
		t.Fatalf("restored samples: got %d want 3", n)
	}
}

func TestRestoreCommandMergeAddsOnlyMissing(t *testing.T) {
	printSampleConfig = false
	dir := t.TempDir()
	base := time.Now().UTC().Truncate(time.Second)

	srcPath := filepath.Join(dir, "src.db")
	seedStoreFile(t, srcPath, base, "a", "b")
	bak := filepath.Join(dir, "bak.db")
	backupOf(t, srcPath, bak)

	// The destination already holds pod "a" at the same tick.
	dstPath := filepath.Join(dir, "dst.db")
	seedStoreFile(t, dstPath, base, "a")
	cfgPath := writeMiniConfig(t, dir, dstPath)

	run := func() string {
		out := &bytes.Buffer{}
		cmd := NewRootCmd()
		cmd.SetOut(out)
		cmd.SetArgs([]string{"restore", "--config", cfgPath, "--merge", bak})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("merge: %v", err)
		}
		return out.String()
	}

	if got := run(); !strings.Contains(got, "samples         +1") {
		t.Fatalf("first merge should add only the missing tick: %q", got)
	}
	if got := run(); !strings.Contains(got, "samples         +0") {
		t.Fatalf("second merge must add nothing: %q", got)
	}
	if n := countSamples(t, dstPath); n != 2 {
		t.Fatalf("merged samples: got %d want 2", n)
	}
}

func TestRestoreCommandRefusesNonEmptyAndRequiresArg(t *testing.T) {
	printSampleConfig = false
	dir := t.TempDir()
	base := time.Now().UTC().Truncate(time.Second)

	srcPath := filepath.Join(dir, "src.db")
	seedStoreFile(t, srcPath, base, "a")
	bak := filepath.Join(dir, "bak.db")
	backupOf(t, srcPath, bak)

	// Destination already holds a store with data.
	dstPath := filepath.Join(dir, "dst.db")
	seedStoreFile(t, dstPath, base, "keep-me")
	cfgPath := writeMiniConfig(t, dir, dstPath)

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"restore", "--config", cfgPath, bak})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected restore to refuse replacing a non-empty store")
	}

	cmd = NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"restore", "--config", cfgPath, "--force", bak})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("forced restore: %v", err)
	}
	if n := countSamples(t, dstPath); n != 1 {
		t.Fatalf("forced restore samples: got %d want 1", n)
	}

	cmd = NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"restore", "--config", cfgPath})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected restore to require a backup path")
	}
}
