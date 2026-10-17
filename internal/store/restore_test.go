package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func restoreRec(at time.Time, pod string) Record {
	return Record{
		SampledAt:        at,
		ClientID:         "c1",
		ClusterName:      "prod",
		Namespace:        "app",
		Pod:              pod,
		Container:        "web",
		CPUUtilMcores:    100,
		CPURequestMcores: 200,
	}
}

// makeBackup seeds a store and returns a VACUUM INTO copy of it.
func makeBackup(t *testing.T, dir string) string {
	t.Helper()
	src, err := Open(filepath.Join(dir, "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	if err := src.InsertRecords(t.Context(), []Record{
		restoreRec(now, "web-0"),
		restoreRec(now.Add(time.Minute), "web-1"),
	}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "backup.db")
	if err := src.Backup(t.Context(), out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Merge adds only what the target lacks, and a second run of the same file adds
// nothing -- that idempotence is what makes merge safe to repeat.
func TestMergeAddsMissingRowsOnce(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	src, err := Open(filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := src.InsertRecords(t.Context(), []Record{
		restoreRec(now, "web-0"),
		restoreRec(now.Add(time.Minute), "web-0"),
	}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "backup.db")
	if err := src.Backup(t.Context(), backup); err != nil {
		t.Fatal(err)
	}
	_ = src.Close()

	dst, err := Open(filepath.Join(dir, "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	// The target already holds the first tick under a different row id.
	if err := dst.InsertRecords(t.Context(), []Record{restoreRec(now, "web-0")}); err != nil {
		t.Fatal(err)
	}

	rep, err := dst.MergeFrom(t.Context(), backup)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if rep.Samples != 1 {
		t.Fatalf("merged samples: got %d want 1 (only the missing tick)", rep.Samples)
	}

	rep, err = dst.MergeFrom(t.Context(), backup)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if rep.Total() != 0 {
		t.Fatalf("second merge added %d rows; merge must be idempotent", rep.Total())
	}

	var n int
	if err := dst.db.QueryRow("SELECT COUNT(*) FROM samples").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("target rows: got %d want 2", n)
	}
}

// A backup from before a table or column existed still merges: absent tables are
// skipped and absent columns keep the target's defaults.
func TestMergeAcceptsOlderBackup(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.db")

	raw, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE schema_meta (version INTEGER NOT NULL)`,
		`INSERT INTO schema_meta(version) VALUES (4)`,
		// v6 samples has mem_limit_bytes; this v4 copy does not.
		`CREATE TABLE samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sampled_at TEXT NOT NULL,
			client_id TEXT NOT NULL DEFAULT '',
			cluster_name TEXT NOT NULL DEFAULT '',
			namespace TEXT NOT NULL,
			pod TEXT NOT NULL,
			container TEXT NOT NULL,
			owner_kind TEXT NOT NULL DEFAULT '',
			owner_name TEXT NOT NULL DEFAULT '',
			node TEXT NOT NULL DEFAULT '',
			cpu_util_mcores REAL NOT NULL DEFAULT 0,
			mem_util_bytes INTEGER NOT NULL DEFAULT 0,
			cpu_request_mcores REAL NOT NULL DEFAULT 0,
			mem_request_bytes INTEGER NOT NULL DEFAULT 0,
			cpu_limit_mcores REAL NOT NULL DEFAULT 0
		)`,
		`INSERT INTO samples (sampled_at, client_id, cluster_name, namespace, pod, container)
		 VALUES ('2026-01-01T00:00:00Z','c1','prod','app','web-0','web')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	dst, err := Open(filepath.Join(dir, "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()

	rep, err := dst.MergeFrom(t.Context(), old)
	if err != nil {
		t.Fatalf("merge old backup: %v", err)
	}
	if rep.Samples != 1 {
		t.Fatalf("samples: got %d want 1", rep.Samples)
	}
	if rep.SchemaVersion != 4 {
		t.Fatalf("schema: got %d want 4", rep.SchemaVersion)
	}
	// Neither table exists in a v4 backup, and neither is an error.
	if rep.Recommendations != 0 || rep.NodeSnapshots != 0 {
		t.Fatalf("rows reported from absent tables: %+v", rep)
	}
	var memLimit int64
	if err := dst.db.QueryRow("SELECT mem_limit_bytes FROM samples").Scan(&memLimit); err != nil {
		t.Fatalf("newer column should default, not fail: %v", err)
	}
	if memLimit != 0 {
		t.Fatalf("mem_limit_bytes: got %d want 0", memLimit)
	}
}

// A backup from the future is refused: reading it could be wrong and migrating it
// silently would be worse.
func TestMergeRejectsNewerSchema(t *testing.T) {
	dir := t.TempDir()
	future := filepath.Join(dir, "future.db")

	raw, err := sql.Open("sqlite", future)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE schema_meta (version INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_meta(version) VALUES (?)`, SchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	dst, err := Open(filepath.Join(dir, "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := dst.MergeFrom(t.Context(), future); err == nil {
		t.Fatal("expected a newer-schema backup to be refused")
	}
	if _, err := ValidateBackup(future); err == nil {
		t.Fatal("expected ValidateBackup to refuse a newer schema")
	}
}

// Replace refuses to clobber a store that holds data unless forced.
func TestRestoreReplaceRefusesNonEmptyWithoutForce(t *testing.T) {
	dir := t.TempDir()
	backup := makeBackup(t, dir)
	dstPath := filepath.Join(dir, "dst.db")

	live, err := Open(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := live.InsertRecords(t.Context(), []Record{restoreRec(time.Now().UTC(), "keep-me")}); err != nil {
		t.Fatal(err)
	}
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}

	if err := RestoreReplace(backup, dstPath, false); err == nil {
		t.Fatal("expected refusal to replace a non-empty store without --force")
	}
	if err := RestoreReplace(backup, dstPath, true); err != nil {
		t.Fatalf("force replace: %v", err)
	}

	restored, err := OpenReadOnly(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	var n int
	if err := restored.db.QueryRow("SELECT COUNT(*) FROM samples").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("restored rows: got %d want 2 (the backup's)", n)
	}
}

// A -wal left beside the destination belongs to the database being replaced.
// Leaving it behind is how a hand-copied store ends up corrupt.
func TestRestoreReplaceClearsStaleWal(t *testing.T) {
	dir := t.TempDir()
	backup := makeBackup(t, dir)
	dstPath := filepath.Join(dir, "dst.db")

	stale := dstPath + "-wal"
	if err := os.WriteFile(stale, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreReplace(backup, dstPath, false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale -wal should have been removed, stat err = %v", err)
	}
}

func TestRestoreReplaceRejectsSameFileAndNonStore(t *testing.T) {
	dir := t.TempDir()
	backup := makeBackup(t, dir)

	if err := RestoreReplace(backup, backup, true); err == nil {
		t.Fatal("expected same-file rejection")
	}

	junk := filepath.Join(dir, "junk.db")
	if err := os.WriteFile(junk, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBackup(junk); err == nil {
		t.Fatal("expected a non-store file to be rejected")
	}
	if err := RestoreReplace(junk, filepath.Join(dir, "x.db"), true); err == nil {
		t.Fatal("expected restore to refuse a non-store source")
	}
}
