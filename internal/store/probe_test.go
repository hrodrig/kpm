package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A diagnostic must not create the store it was asked to inspect: Open() would,
// and would then report a healthy store that never existed.
func TestOpenReadOnlyMissingFileDoesNotCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")

	if _, err := OpenReadOnly(path); err == nil {
		t.Fatal("expected an error for a missing store")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly created the file (stat err = %v)", err)
	}
}

// Opening read-only must not migrate: the whole point is to report the version
// that is actually on disk.
func TestOpenReadOnlyDoesNotMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Pretend the file was written by an older build.
	if _, err := s.db.Exec(`UPDATE schema_meta SET version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer func() { _ = ro.Close() }()

	ver, err := ro.SchemaVersionOf(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ver != 1 {
		t.Fatalf("read-only open changed the schema version: got v%d want v1", ver)
	}
}

func TestSampleBoundsAndCoverageOnEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	oldest, newest, err := s.SampleBounds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !oldest.IsZero() || !newest.IsZero() {
		t.Fatalf("empty store bounds: %v → %v", oldest, newest)
	}

	days, err := s.DaysWithSamplesSince(ctx, time.Now().UTC().AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Fatalf("empty store coverage: got %d want 0", days)
	}
}

func TestSampleBoundsAndCoverageCountDistinctDays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	// Three days, two samples each: the day count must be 3, not 6.
	recs := []Record{}
	for i := range 3 {
		at := now.AddDate(0, 0, -i)
		for j := range 2 {
			recs = append(recs, Record{
				SampledAt:     at.Add(time.Duration(j) * time.Hour),
				Namespace:     "app",
				Pod:           "p",
				Container:     "c",
				OwnerKind:     "Deployment",
				OwnerName:     "web",
				CPUUtilMcores: 10,
				MemUtilBytes:  1 << 20,
			})
		}
	}
	if err := s.InsertRecords(ctx, recs); err != nil {
		t.Fatal(err)
	}

	oldest, newest, err := s.SampleBounds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !oldest.Equal(now.AddDate(0, 0, -2)) {
		t.Fatalf("oldest: got %v want %v", oldest, now.AddDate(0, 0, -2))
	}
	if !newest.Equal(now.Add(time.Hour)) {
		t.Fatalf("newest: got %v want %v", newest, now.Add(time.Hour))
	}

	days, err := s.DaysWithSamplesSince(ctx, now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	if days != 3 {
		t.Fatalf("covered days: got %d want 3", days)
	}

	// A window that starts after the oldest day must not count it.
	days, err = s.DaysWithSamplesSince(ctx, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatal(err)
	}
	if days != 2 {
		t.Fatalf("narrowed window: got %d want 2", days)
	}
}
