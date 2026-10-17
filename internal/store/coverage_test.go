package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func recommendationHasColumn(t *testing.T, s *Store, column string) bool {
	t.Helper()
	rows, err := s.db.Query("PRAGMA table_info(recommendations)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

// #26: databases created before the coverage column exist with the old
// recommendations table, and migrate() is CREATE TABLE IF NOT EXISTS — a no-op
// on an existing table. Without the ALTER path those DBs would fail on every
// read and write of the new column.
func TestMigrateAddsDaysWithSamplesColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	ctx := context.Background()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-#26 database.
	if _, err := s.db.Exec("ALTER TABLE recommendations DROP COLUMN days_with_samples"); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if recommendationHasColumn(t, s, "days_with_samples") {
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

	if !recommendationHasColumn(t, s2, "days_with_samples") {
		t.Fatal("migration did not restore the column")
	}

	at := time.Now().UTC().Truncate(time.Second)
	if err := s2.InsertRecommendations(ctx, []Recommendation{{
		RecordedAt: at, Grain: "workload", GrainKey: "Deployment/web/app",
		WindowDays: 7, DaysWithSamples: 3,
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
	if got.DaysWithSamples != 3 {
		t.Fatalf("days_with_samples round-trip: got %d want 3", got.DaysWithSamples)
	}
	if got.WindowDays != 7 {
		t.Fatalf("window_days round-trip: got %d want 7", got.WindowDays)
	}
}

// Coverage travels with the whole snapshot list too, not only the single-row read.
func TestLatestRecommendationsCarryCoverage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "list.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)
	if err := s.InsertRecommendations(ctx, []Recommendation{{
		RecordedAt: at, Grain: "pod", GrainKey: "app/p/app",
		WindowDays: 7, DaysWithSamples: 4,
	}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.LatestRecommendations(ctx, "pod")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d want 1", len(rows))
	}
	if rows[0].DaysWithSamples != 4 {
		t.Fatalf("list coverage: got %d want 4", rows[0].DaysWithSamples)
	}
}

// A row whose writer never set coverage reads as "unknown" (0), not as a
// measurement of zero covered days.
func TestRecommendationCoverageDefaultsToUnknown(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	if err := s.InsertRecommendations(ctx, []Recommendation{{
		RecordedAt: time.Now().UTC().Truncate(time.Second),
		Grain:      "workload", GrainKey: "Deployment/old/app",
		WindowDays: 7,
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
	if got.DaysWithSamples != 0 {
		t.Fatalf("unset coverage: got %d want 0", got.DaysWithSamples)
	}
}
