package store

import (
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
