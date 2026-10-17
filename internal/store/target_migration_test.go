package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func tableColumns(t *testing.T, s *Store, table string) map[string]bool {
	t.Helper()
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return cols
}

// #25: databases created before the per-resource targets exist with the old
// recommendations table, and migrate() is CREATE TABLE IF NOT EXISTS — a no-op
// on an existing table. Without the ALTER path those DBs would fail on every
// read and write of the new columns.
func TestMigrateAddsPerResourceTargetColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	ctx := context.Background()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-#25 database.
	for _, col := range []string{"cpu_target_utilization", "memory_target_utilization"} {
		if _, err := s.db.Exec("ALTER TABLE recommendations DROP COLUMN " + col); err != nil {
			t.Fatalf("drop %s: %v", col, err)
		}
	}
	if cols := tableColumns(t, s, "recommendations"); cols["cpu_target_utilization"] {
		t.Fatal("precondition failed: column still present")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s2.Close() }()

	cols := tableColumns(t, s2, "recommendations")
	if !cols["cpu_target_utilization"] || !cols["memory_target_utilization"] {
		t.Fatalf("migration did not restore the columns: %v", cols)
	}

	at := time.Now().UTC().Truncate(time.Second)
	if err := s2.InsertRecommendations(ctx, []Recommendation{{
		RecordedAt: at, Grain: "workload", GrainKey: "Deployment/web/app",
		TargetUtilization: 0.70, CPUTargetUtilization: 0.50, MemoryTargetUtilization: 0.90,
	}}); err != nil {
		t.Fatalf("insert after migration: %v", err)
	}
	got, err := s2.LatestRecommendation(ctx, "workload", "Deployment/web/app")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil recommendation")
	}
	if got.CPUTarget() != 0.50 || got.MemTarget() != 0.90 {
		t.Fatalf("round-trip targets: cpu=%v mem=%v", got.CPUTarget(), got.MemTarget())
	}
}

// A fresh database must reach SchemaVersion and carry the columns immediately.
func TestFreshStoreHasPerResourceTargetColumns(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	cols := tableColumns(t, s, "recommendations")
	if !cols["cpu_target_utilization"] || !cols["memory_target_utilization"] {
		t.Fatalf("fresh store missing columns: %v", cols)
	}
	var ver int
	if err := s.db.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion {
		t.Fatalf("schema version %d, want %d", ver, SchemaVersion)
	}
}

// Legacy rows (both per-resource columns 0) must keep scoring against the shared
// target rather than reading a zero target.
func TestLegacyRecommendationFallsBackToSharedTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	ctx := context.Background()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	at := time.Now().UTC().Truncate(time.Second)
	if err := s.InsertRecommendations(ctx, []Recommendation{{
		RecordedAt: at, Grain: "workload", GrainKey: "Deployment/old/app",
		TargetUtilization: 0.70,
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.LatestRecommendation(ctx, "workload", "Deployment/old/app")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil recommendation")
	}
	if got.CPUTarget() != 0.70 || got.MemTarget() != 0.70 {
		t.Fatalf("legacy fallback: cpu=%v mem=%v", got.CPUTarget(), got.MemTarget())
	}
}
