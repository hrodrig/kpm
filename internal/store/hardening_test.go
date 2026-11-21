package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMergePreservesNodeSnapshotTaintsAndUnschedulable(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

	src, err := Open(filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := src.InsertNodeSnapshots(t.Context(), []NodeSnapshot{{
		RecordedAt:     now,
		ClientID:       "c1",
		ClusterName:    "prod",
		Node:           "gpu-1",
		CPUAllocMcores: 8000,
		MemAllocBytes:  32 << 30,
		Pods:           4,
		Taints:         []string{"nvidia.com/gpu=true:NoSchedule"},
		Unschedulable:  true,
	}}); err != nil {
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

	rep, err := dst.MergeFrom(t.Context(), backup)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NodeSnapshots != 1 {
		t.Fatalf("want 1 merged node snapshot, got %d", rep.NodeSnapshots)
	}
	got, err := dst.LatestNodeSnapshots(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 node snapshot, got %d", len(got))
	}
	if !got[0].Unschedulable {
		t.Fatal("expected Unschedulable=true preserved after MergeFrom")
	}
	if len(got[0].Taints) != 1 || got[0].Taints[0] != "nvidia.com/gpu=true:NoSchedule" {
		t.Fatalf("expected taints preserved, got %v", got[0].Taints)
	}
}

func TestMergeNormalizesLegacyRFC3339Timestamps(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	src, err := Open(filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	// Insert a legacy RFC3339 row (without .000000000) directly.
	if _, err := src.db.Exec(`INSERT INTO samples (
		sampled_at, client_id, cluster_name, namespace, pod, container
	) VALUES (?, 'c1', 'prod', 'app', 'web-0', 'web')`, ts.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "legacy.db")
	if err := src.Backup(t.Context(), backup); err != nil {
		t.Fatal(err)
	}
	_ = src.Close()

	// Target already has the exact same sample stored in canonical UTCLayout.
	dst, err := Open(filepath.Join(dir, "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	if err := dst.InsertRecords(t.Context(), []Record{restoreRec(ts, "web-0")}); err != nil {
		t.Fatal(err)
	}

	rep, err := dst.MergeFrom(t.Context(), backup)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Samples != 0 {
		t.Fatalf("legacy RFC3339 row should dedupe against UTCLayout row, added=%d", rep.Samples)
	}

	// Merging into an empty target stores canonical UTCLayout.
	dst2, err := Open(filepath.Join(dir, "dst2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst2.Close() }()
	rep2, err := dst2.MergeFrom(t.Context(), backup)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Samples != 1 {
		t.Fatalf("want 1 sample merged into empty target, got %d", rep2.Samples)
	}
	var stored string
	if err := dst2.db.QueryRow(`SELECT sampled_at FROM samples LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != FormatUTC(ts) {
		t.Fatalf("want normalized UTCLayout %q, got %q", FormatUTC(ts), stored)
	}
}

func TestRestoreReplaceIncludesUncheckpointedWAL(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "live.db")
	src, err := Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()

	// Disable automatic checkpoints so inserted rows remain in live.db-wal.
	if _, err := src.db.Exec(`PRAGMA wal_autocheckpoint = 0`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := src.InsertRecords(t.Context(), []Record{
		restoreRec(now, "wal-pod-0"),
		restoreRec(now.Add(time.Minute), "wal-pod-1"),
	}); err != nil {
		t.Fatal(err)
	}

	walInfo, err := os.Stat(srcPath + "-wal")
	if err != nil || walInfo.Size() == 0 {
		t.Fatalf("expected non-empty -wal sidecar, stat err=%v", err)
	}

	dstPath := filepath.Join(dir, "restored.db")
	if err := RestoreReplace(srcPath, dstPath, false); err != nil {
		t.Fatalf("RestoreReplace from live WAL source: %v", err)
	}

	fi, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("restored file mode %o wants 0600", fi.Mode().Perm())
	}

	dst, err := OpenReadOnly(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	n, err := dst.Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("restored store lost uncheckpointed WAL rows: got %d want 2", n)
	}
}

func TestOpenDoesNotChmodExistingParentDir(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(parent, "kpm.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	fi, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("existing parent dir mode changed to %o, want 0755", fi.Mode().Perm())
	}

	// Newly created dir still gets 0700.
	nested := filepath.Join(parent, "fresh-subdir")
	st2, err := Open(filepath.Join(nested, "kpm.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = st2.Close()
	fi2, err := os.Stat(nested)
	if err != nil {
		t.Fatal(err)
	}
	if fi2.Mode().Perm() != 0o700 {
		t.Fatalf("new parent dir mode %o, want 0700", fi2.Mode().Perm())
	}
}

func TestLatestNodeSnapshotsSurfacesErrors(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	// Empty table returns nil, nil.
	rows, err := st.LatestNodeSnapshots(t.Context())
	if err != nil || len(rows) != 0 {
		t.Fatalf("empty table: rows=%d err=%v", len(rows), err)
	}
	// Canceled context must return an error, not nil, nil.
	cctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := st.LatestNodeSnapshots(cctx); err == nil {
		t.Fatal("expected canceled context error from LatestNodeSnapshots")
	}
	_ = st.Close()
	if _, err := st.LatestNodeSnapshots(t.Context()); err == nil {
		t.Fatal("expected closed DB error from LatestNodeSnapshots")
	}
}

func TestLatestRecommendationsTieBreaksByID(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	ts := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	if err := st.InsertRecommendations(t.Context(), []Recommendation{
		{
			RecordedAt: ts, Grain: "workload", GrainKey: "Deployment/api/app",
			Namespace: "app", OwnerKind: "Deployment", OwnerName: "api", Container: "app",
			CPUSugMcores: 100,
		},
		{
			RecordedAt: ts, Grain: "workload", GrainKey: "Deployment/api/app",
			Namespace: "app", OwnerKind: "Deployment", OwnerName: "api", Container: "app",
			CPUSugMcores: 250,
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.LatestRecommendations(t.Context(), "workload")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 deduplicated recommendation on timestamp tie, got %d", len(got))
	}
	if got[0].CPUSugMcores != 250 {
		t.Fatalf("want newest id (CPUSugMcores=250), got %g", got[0].CPUSugMcores)
	}
}
