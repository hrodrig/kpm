package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestNodeSnapshotPoolRoundtrip(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	at := time.Now().UTC().Truncate(time.Second)
	if err := st.InsertNodeSnapshots(t.Context(), []NodeSnapshot{
		{RecordedAt: at, ClientID: "c", ClusterName: "prod", Node: "gpu-0",
			Taints: []string{"dedicated=gpu:NoSchedule", "spot:NoExecute"}, Unschedulable: true},
		{RecordedAt: at, ClientID: "c", ClusterName: "prod", Node: "gen-0"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := st.LatestNodeSnapshots(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rows: got %d want 2", len(got))
	}
	// ORDER BY node: gen-0, then gpu-0.
	if len(got[0].Taints) != 0 || got[0].Unschedulable {
		t.Fatalf("general node should have no pool: %+v", got[0])
	}
	if len(got[1].Taints) != 2 || got[1].Taints[0] != "dedicated=gpu:NoSchedule" {
		t.Fatalf("taints: %v", got[1].Taints)
	}
	if !got[1].Unschedulable {
		t.Fatal("cordoned node lost its flag")
	}
}

// writeV6Store builds a store shaped like schema v6: node_snapshots without the
// pool columns, and one row already in it.
func writeV6Store(t *testing.T, path string) {
	t.Helper()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE schema_meta (version INTEGER NOT NULL)`,
		`INSERT INTO schema_meta(version) VALUES (6)`,
		`CREATE TABLE node_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			client_id TEXT NOT NULL DEFAULT '',
			cluster_name TEXT NOT NULL DEFAULT '',
			node TEXT NOT NULL,
			cpu_alloc_mcores REAL NOT NULL DEFAULT 0,
			mem_alloc_bytes INTEGER NOT NULL DEFAULT 0,
			cpu_cap_mcores REAL NOT NULL DEFAULT 0,
			mem_cap_bytes INTEGER NOT NULL DEFAULT 0,
			cpu_req_mcores REAL NOT NULL DEFAULT 0,
			mem_req_bytes INTEGER NOT NULL DEFAULT 0,
			cpu_util_mcores REAL NOT NULL DEFAULT 0,
			mem_util_bytes INTEGER NOT NULL DEFAULT 0,
			pods INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO node_snapshots (recorded_at, node, cpu_alloc_mcores)
		 VALUES ('2026-01-01T00:00:00Z', 'old-0', 2000)`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
}

// A store written before v7 has the table without the pool columns. Opening it
// must add them, and rows written afterwards must carry pool identity.
func TestNodeSnapshotMigrationAddsPoolColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.db")
	writeV6Store(t, path)

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open v6 store: %v", err)
	}
	defer func() { _ = st.Close() }()

	ver, err := st.SchemaVersionOf(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion {
		t.Fatalf("schema: got %d want %d", ver, SchemaVersion)
	}

	// The pre-v7 row survives with empty pool identity...
	old, err := st.LatestNodeSnapshots(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(old) != 1 || old[0].Node != "old-0" {
		t.Fatalf("pre-v7 row: %+v", old)
	}
	if len(old[0].Taints) != 0 || old[0].Unschedulable {
		t.Fatalf("pre-v7 row should default to no pool: %+v", old[0])
	}

	// ...and new rows can carry it.
	at := time.Now().UTC().Truncate(time.Second)
	if err := st.InsertNodeSnapshots(t.Context(), []NodeSnapshot{{
		RecordedAt: at, Node: "new-0", Taints: []string{"dedicated=gpu:NoSchedule"},
	}}); err != nil {
		t.Fatalf("insert after migration: %v", err)
	}
	latest, err := st.LatestNodeSnapshots(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 || latest[0].Node != "new-0" {
		t.Fatalf("latest batch: %+v", latest)
	}
	if len(latest[0].Taints) != 1 {
		t.Fatalf("taints after migration: %v", latest[0].Taints)
	}
}
