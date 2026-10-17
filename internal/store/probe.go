package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// OpenReadOnly opens an existing store without creating it and without running
// migrations.
//
// Open() creates a missing file, chmods it and migrates the schema — all writes.
// A diagnostic must observe the store as it is: creating a file would report a
// healthy store that never existed, and migrating would silently "fix" the
// version it was asked to inspect.
func OpenReadOnly(path string) (*Store, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat store: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("store path %q is a directory", path)
	}

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	u.RawQuery = "mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db, path: path}, nil
}

// SchemaVersionOf reports the schema revision recorded in the open database.
func (s *Store) SchemaVersionOf(ctx context.Context) (int, error) {
	var ver int
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_meta LIMIT 1`).Scan(&ver); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return ver, nil
}

// SampleBounds returns the oldest and newest sampled_at in the store. Both are
// zero when the store holds no samples.
func (s *Store) SampleBounds(ctx context.Context) (time.Time, time.Time, error) {
	var oldest, newest *string
	err := s.db.QueryRowContext(ctx,
		`SELECT MIN(sampled_at), MAX(sampled_at) FROM samples`).Scan(&oldest, &newest)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("read sample bounds: %w", err)
	}
	if oldest == nil || newest == nil {
		return time.Time{}, time.Time{}, nil
	}
	from, err := ParseUTC(*oldest)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse oldest sample %q: %w", *oldest, err)
	}
	to, err := ParseUTC(*newest)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse newest sample %q: %w", *newest, err)
	}
	return from, to, nil
}

// DaysWithSamplesSince counts the distinct UTC days holding at least one sample
// at or after since.
//
// sampled_at is stored as fixed-width UTC text (§2.2), so its first ten
// characters are the UTC date and lexicographic comparison matches time order.
// This counts the same days recommend.Run counts over the same window, without
// loading the records themselves.
func (s *Store) DaysWithSamplesSince(ctx context.Context, since time.Time) (int, error) {
	var days int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT substr(sampled_at, 1, 10)) FROM samples WHERE sampled_at >= ?`,
		FormatUTC(since),
	).Scan(&days)
	if err != nil {
		return 0, fmt.Errorf("count covered days: %w", err)
	}
	return days, nil
}
