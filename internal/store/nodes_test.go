package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNodeSnapshotsInsertLatest(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var ver int
	if err := st.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion {
		t.Fatalf("schema %d want %d", ver, SchemaVersion)
	}
	var name string
	if err := st.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='node_snapshots'`,
	).Scan(&name); err != nil || name != "node_snapshots" {
		t.Fatalf("table: %v %q", err, name)
	}

	t1 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	if err := st.InsertNodeSnapshots(t.Context(), []NodeSnapshot{
		{RecordedAt: t1, ClientID: "c", ClusterName: "x", Node: "n1", CPUAllocMcores: 1000, CPUReqMcores: 100, Pods: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertNodeSnapshots(t.Context(), []NodeSnapshot{
		{RecordedAt: t2, ClientID: "c", ClusterName: "x", Node: "n1", CPUAllocMcores: 1000, CPUReqMcores: 200, Pods: 2},
		{RecordedAt: t2, ClientID: "c", ClusterName: "x", Node: "n2", CPUAllocMcores: 2000, CPUReqMcores: 50, Pods: 1},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.LatestNodeSnapshots(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if !got[0].RecordedAt.Equal(t2) || got[0].Node != "n1" || got[0].CPUReqMcores != 200 {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].Node != "n2" {
		t.Fatalf("second %+v", got[1])
	}
}
