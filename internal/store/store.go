// Package store persists container metrics samples in SQLite.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SchemaVersion is the current on-disk schema revision.
// 5 adds per-resource recommend targets (#25); 6 adds window coverage (#26);
// 7 adds node pool identity (blocking taints and cordon state).
const SchemaVersion = 7

// MaxScanRows is the hard upper bound for a single ListRecords call when
// callers pass Limit=0 (uncapped APIs must set their own Limit). Documented
// fail-closed ceiling for analyze-local / recommend safety nets.
const MaxScanRows = 100_000

// Record is one container sample at a point in time.
type Record struct {
	SampledAt        time.Time
	ClientID         string // resolved sampler identity
	ClusterName      string // config cluster.name
	Namespace        string
	Pod              string
	Container        string
	OwnerKind        string
	OwnerName        string
	Node             string
	CPUUtilMcores    float64
	MemUtilBytes     int64
	CPURequestMcores float64
	MemRequestBytes  int64
	CPULimitMcores   float64
	MemLimitBytes    int64
}

// readPoolConns is how many concurrent SELECTs may run beside the writer.
// The writer keeps its own connection (MaxOpenConns 1) so a long API scan
// cannot hold the sampler's insert.
const readPoolConns = 4

// Store wraps an open SQLite database.
type Store struct {
	db   *sql.DB // writes and migrations; one connection
	read *sql.DB // SELECTs; nil on OpenReadOnly, which has only db
	path string
}

// reads returns the connection pool for SELECTs. OpenReadOnly has no separate
// pool, so those calls stay on the single read-only connection.
func (s *Store) reads() *sql.DB {
	if s.read != nil {
		return s.read
	}
	return s.db
}

// Open creates parent dirs, opens the DB, and migrates schema.
// Directory mode is 0700; the DB file is created at 0600 before sql.Open so
// WAL sidecars inherit owner-only mode. WAL + busy_timeout reduce
// "database is locked" when sample and serve share a file.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	_ = os.Chmod(dir, 0o700)

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create store file: %w", err)
	}
	_ = f.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("chmod store: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := chmodStoreFiles(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	read, err := sql.Open("sqlite", sqliteReadDSN(path))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite read pool: %w", err)
	}
	read.SetMaxOpenConns(readPoolConns)
	s.read = read
	return s, nil
}

func sqliteDSN(path string) string {
	// file: URL so paths containing ? or # do not break pragma query parsing.
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	u.RawQuery = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	return u.String()
}

// sqliteReadDSN opens the same file for SELECTs only. journal_mode is not set
// here: that pragma writes, and the writer in sqliteDSN already put the file
// in WAL mode, which is what lets this pool run beside an insert.
func sqliteReadDSN(path string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	u.RawQuery = "mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	return u.String()
}

func chmodStoreFiles(path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("chmod store %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.read != nil {
		err = s.read.Close()
	}
	if s.db != nil {
		if cerr := s.db.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// Path returns the on-disk path.
func (s *Store) Path() string { return s.path }

func (s *Store) migrate() error {
	// schema_meta comes first, and its version is read *before* any other DDL: a
	// store written by a newer build has to be refused while it is still untouched.
	// An older binary adding what merely looks missing to it would corrupt a schema
	// it cannot read.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (
			version INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	empty, ver, err := s.schemaState()
	if err != nil {
		return err
	}
	if !empty && ver > SchemaVersion {
		return fmt.Errorf(
			"store schema v%d is newer than this build understands (v%d): upgrade kpm", ver, SchemaVersion)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS samples (
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
			cpu_limit_mcores REAL NOT NULL DEFAULT 0,
			mem_limit_bytes INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_sampled_at ON samples(sampled_at)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_ns_pod ON samples(namespace, pod)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_ns_owner_at ON samples(namespace, owner_name, sampled_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if err := migrateRecommendations(s.db); err != nil {
		return err
	}
	if err := migrateNodeSnapshots(s.db); err != nil {
		return err
	}

	if empty {
		_, err := s.db.Exec(`INSERT INTO schema_meta(version) VALUES (?)`, SchemaVersion)
		return err
	}
	if ver < SchemaVersion {
		_, err := s.db.Exec(`UPDATE schema_meta SET version = ?`, SchemaVersion)
		return err
	}
	return nil
}

// schemaState reports whether schema_meta carries a version yet, and which.
func (s *Store) schemaState() (empty bool, version int, err error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_meta`).Scan(&n); err != nil {
		return false, 0, err
	}
	if n == 0 {
		return true, 0, nil
	}
	if err := s.db.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&version); err != nil {
		return false, 0, err
	}
	return false, version, nil
}

// InsertRecords writes records in a single transaction.
func (s *Store) InsertRecords(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO samples (
			sampled_at, client_id, cluster_name,
			namespace, pod, container,
			owner_kind, owner_name, node,
			cpu_util_mcores, mem_util_bytes,
			cpu_request_mcores, mem_request_bytes,
			cpu_limit_mcores, mem_limit_bytes
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		at := FormatUTC(r.SampledAt)
		if _, err := stmt.ExecContext(ctx,
			at, r.ClientID, r.ClusterName,
			r.Namespace, r.Pod, r.Container,
			r.OwnerKind, r.OwnerName, r.Node,
			r.CPUUtilMcores, r.MemUtilBytes,
			r.CPURequestMcores, r.MemRequestBytes,
			r.CPULimitMcores, r.MemLimitBytes,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Count returns the number of sample rows.
func (s *Store) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.reads().QueryRowContext(ctx, `SELECT COUNT(*) FROM samples`).Scan(&n)
	return n, err
}

// PruneOlderThan deletes samples older than cutoff (UTC).
func (s *Store) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM samples WHERE sampled_at < ?`,
		FormatUTC(cutoff),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneRetention deletes samples older than retentionDays from now.
func (s *Store) PruneRetention(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays < 1 {
		return 0, fmt.Errorf("retentionDays must be >= 1")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	return s.PruneOlderThan(ctx, cutoff)
}
