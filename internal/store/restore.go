package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BatchSize is how many rows a merge stages per transaction. Small enough to
// keep memory flat on a large backup, large enough that a 100k-row store does
// not pay 100k round trips.
const BatchSize = 500

// MergeReport says what a merge actually moved. The counts are rows added to the
// target, not rows read from the source: a second merge of the same file adds
// zero, which is the property that makes merge safe to repeat.
type MergeReport struct {
	Source          string `json:"source"`
	SchemaVersion   int    `json:"schema_version"`
	Samples         int64  `json:"samples"`
	Recommendations int64  `json:"recommendations"`
	NodeSnapshots   int64  `json:"node_snapshots"`
}

// Total is the number of rows added across all tables.
func (r MergeReport) Total() int64 {
	return r.Samples + r.Recommendations + r.NodeSnapshots
}

// mergeSpec describes how one table is merged.
type mergeSpec struct {
	table string
	cols  []string
	key   []string
}

// mergeSpecs lists the tables a merge covers and the natural key each one
// dedupes on.
//
// samples has no UNIQUE constraint -- only an autoincrement id -- so id carries
// no identity across databases. The key is the pair that means "the same
// observation": one sample per container per tick.
var mergeSpecs = []mergeSpec{
	{
		table: "samples",
		cols: []string{
			"sampled_at", "client_id", "cluster_name", "namespace", "pod", "container",
			"owner_kind", "owner_name", "node",
			"cpu_util_mcores", "mem_util_bytes", "cpu_request_mcores",
			"mem_request_bytes", "cpu_limit_mcores", "mem_limit_bytes",
		},
		key: []string{"client_id", "cluster_name", "namespace", "pod", "container", "sampled_at"},
	},
	{
		table: "recommendations",
		cols: []string{
			"recorded_at", "client_id", "cluster_name", "grain", "grain_key",
			"namespace", "pod", "container", "owner_kind", "owner_name",
			"samples", "percentile", "target_utilization",
			"cpu_target_utilization", "memory_target_utilization",
			"window_days", "days_with_samples",
			"cpu_p_mcores", "mem_p_bytes", "cpu_sug_mcores", "mem_sug_bytes",
			"cpu_req_mcores", "mem_req_bytes",
		},
		key: []string{"client_id", "cluster_name", "grain", "grain_key", "recorded_at"},
	},
	{
		table: "node_snapshots",
		cols: []string{
			"recorded_at", "client_id", "cluster_name", "node",
			"cpu_alloc_mcores", "mem_alloc_bytes", "cpu_cap_mcores", "mem_cap_bytes",
			"cpu_req_mcores", "mem_req_bytes", "cpu_util_mcores", "mem_util_bytes",
			"pods", "taints", "unschedulable",
		},
		key: []string{"client_id", "cluster_name", "node", "recorded_at"},
	},
}

// MergeFrom appends the rows of the store at srcPath that this store does not
// already hold.
//
// The source is opened read-only and never migrated, so merging a live mounted
// file is safe and an older backup is accepted as-is: only the columns both
// sides share are copied, and anything added since the backup keeps its default.
func (s *Store) MergeFrom(ctx context.Context, srcPath string) (MergeReport, error) {
	if s == nil || s.db == nil {
		return MergeReport{}, fmt.Errorf("store: not open")
	}
	src, err := OpenReadOnly(srcPath)
	if err != nil {
		return MergeReport{}, err
	}
	defer func() { _ = src.Close() }()

	ver, err := src.SchemaVersionOf(ctx)
	if err != nil {
		return MergeReport{}, err
	}
	if ver > SchemaVersion {
		return MergeReport{}, fmt.Errorf(
			"backup schema v%d is newer than this build understands (v%d); upgrade kpm to merge it",
			ver, SchemaVersion)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return MergeReport{}, fmt.Errorf("pin store connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	rep := MergeReport{Source: srcPath, SchemaVersion: ver}
	for _, spec := range mergeSpecs {
		n, err := mergeTable(ctx, conn, src.db, spec)
		if err != nil {
			return rep, fmt.Errorf("merge %s: %w", spec.table, err)
		}
		switch spec.table {
		case "samples":
			rep.Samples = n
		case "recommendations":
			rep.Recommendations = n
		case "node_snapshots":
			rep.NodeSnapshots = n
		}
	}
	return rep, nil
}

// mergeTable stages the source rows in a TEMP table on the pinned connection
// and then inserts the ones the target lacks in a single statement, so dedupe
// happens inside SQLite rather than by keeping a key set in memory.
func mergeTable(ctx context.Context, dst *sql.Conn, src *sql.DB, spec mergeSpec) (int64, error) {
	present, err := tableExists(ctx, src, spec.table)
	if err != nil {
		return 0, err
	}
	// A backup from before the table existed has nothing to merge here.
	if !present {
		return 0, nil
	}
	cols, err := sharedColumns(ctx, src, spec)
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, nil
	}
	colList := strings.Join(cols, ", ")
	stage := "stage_" + spec.table

	_, _ = dst.ExecContext(ctx, "DROP TABLE IF EXISTS temp."+stage)
	if _, err := dst.ExecContext(ctx,
		"CREATE TEMP TABLE "+stage+" AS SELECT "+colList+" FROM main."+spec.table+" WHERE 0"); err != nil {
		return 0, err
	}
	defer func() { _, _ = dst.ExecContext(context.WithoutCancel(ctx), "DROP TABLE IF EXISTS temp."+stage) }()

	if err := stageRows(ctx, src, spec.table, cols, dst, stage); err != nil {
		return 0, err
	}

	on := make([]string, len(spec.key))
	for i, k := range spec.key {
		on[i] = "m." + k + " = " + stage + "." + k
	}
	q := "INSERT INTO " + spec.table + " (" + colList + ") SELECT " + colList +
		" FROM " + stage + " WHERE NOT EXISTS (SELECT 1 FROM " + spec.table + " m WHERE " +
		strings.Join(on, " AND ") + ")"

	res, err := dst.ExecContext(ctx, q)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// tableExists reports whether the source carries the table at all. Recommendations
// and node_snapshots were added after the first schema, so an old backup can be
// missing them, and a merge should skip those rather than fail.
func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// sharedColumns intersects the spec's columns with what the source actually has.
// An older backup predates columns the target gained, and
// SELECT on a column that is not there would fail the whole merge.
func sharedColumns(ctx context.Context, src *sql.DB, spec mergeSpec) ([]string, error) {
	r, err := src.QueryContext(ctx, "SELECT * FROM "+spec.table+" LIMIT 0")
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	have, err := r.Columns()
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(have))
	for _, c := range have {
		set[c] = struct{}{}
	}
	out := make([]string, 0, len(spec.cols))
	for _, c := range spec.cols {
		if _, ok := set[c]; ok {
			out = append(out, c)
		}
	}
	// A missing key column would silently merge on fewer fields and duplicate
	// rows, so refuse instead of guessing.
	for _, k := range spec.key {
		if _, ok := set[k]; !ok {
			return nil, fmt.Errorf("source is missing key column %q", k)
		}
	}
	return out, nil
}

// stageRows copies every source row into the staging table in batches,
// normalizing legacy RFC3339 timestamps to UTCLayout so deduplication and
// lexicographic ordering stay consistent across mixed-era backups.
func stageRows(ctx context.Context, src *sql.DB, table string, cols []string, dst *sql.Conn, stage string) error {
	rows, err := src.QueryContext(ctx, "SELECT "+strings.Join(cols, ", ")+" FROM "+table)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	insert := "INSERT INTO " + stage + " VALUES (" +
		strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + ")"

	var batch [][]any
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, err := dst.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, vals := range batch {
			if _, err := tx.ExecContext(ctx, insert, vals...); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	timeCol := timestampColumnIndex(cols)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		normalizeTimestampCell(vals, timeCol)
		batch = append(batch, vals)
		if len(batch) >= BatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return flush()
}

func timestampColumnIndex(cols []string) int {
	for i, c := range cols {
		if c == "sampled_at" || c == "recorded_at" {
			return i
		}
	}
	return -1
}

func normalizeTimestampCell(vals []any, idx int) {
	if idx < 0 || idx >= len(vals) {
		return
	}
	var raw string
	switch v := vals[idx].(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return
	}
	if t, err := ParseUTC(raw); err == nil {
		vals[idx] = FormatUTC(t)
	}
}

// ValidateBackup checks that path holds a kpm store this build can read.
func ValidateBackup(path string) (int, error) {
	st, err := OpenReadOnly(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = st.Close() }()
	ver, err := st.SchemaVersionOf(context.Background())
	if err != nil {
		return 0, err
	}
	if ver > SchemaVersion {
		return ver, fmt.Errorf(
			"backup schema v%d is newer than this build understands (v%d); upgrade kpm",
			ver, SchemaVersion)
	}
	return ver, nil
}

// RestoreReplace installs the store at srcPath as the store at destPath.
//
// The source is snapshotted via SQLite VACUUM INTO inside a private 0700 temp
// directory (capturing any uncheckpointed -wal frames safely without mutating
// the source), chmod'd to 0600, and atomically renamed into place. Stale
// -wal/-shm files next to the destination are removed.
//
// The caller must not have the destination open: a writer that still holds the
// old file keeps writing to an inode nothing points at any more.
func RestoreReplace(srcPath, destPath string, force bool) error {
	src, err := filepath.Abs(srcPath)
	if err != nil {
		return err
	}
	dst, err := filepath.Abs(destPath)
	if err != nil {
		return err
	}
	if src == dst {
		return fmt.Errorf("restore source and destination are the same file: %s", dst)
	}
	if _, err := ValidateBackup(src); err != nil {
		return err
	}
	if fi, err := os.Stat(dst); err == nil && fi.Size() > 0 && !force {
		return fmt.Errorf("destination already holds a store: %s (use --force to replace it)", dst)
	}
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(parent, ".kpm-restore-*")
	if err != nil {
		return fmt.Errorf("create restore staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmp := filepath.Join(tmpDir, "restore.db")
	if err := snapshotStoreFile(src, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("install restore: %w", err)
	}
	return removeWALSidecars(dst)
}

func removeWALSidecars(dst string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dst + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", suffix, err)
		}
	}
	return nil
}

// snapshotStoreFile produces a self-contained SQLite copy of src at dst using
// VACUUM INTO on a read-only connection, preserving any uncheckpointed WAL data.
func snapshotStoreFile(src, dst string) error {
	st, err := OpenReadOnly(src)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer func() { _ = st.Close() }()

	if _, err := st.db.ExecContext(context.Background(), `VACUUM INTO ?`, dst); err != nil {
		return fmt.Errorf("snapshot backup: %w", err)
	}
	if err := os.Chmod(dst, 0o600); err != nil {
		return fmt.Errorf("chmod restore: %w", err)
	}
	return nil
}
