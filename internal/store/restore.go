package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
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
			"pods",
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

	rep := MergeReport{Source: srcPath, SchemaVersion: ver}
	for _, spec := range mergeSpecs {
		n, err := s.mergeTable(ctx, src.db, spec)
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

// mergeTable stages the source rows in a TEMP table and then inserts the ones the
// target lacks in a single statement, so dedupe happens inside SQLite rather than
// by keeping a key set in memory.
func (s *Store) mergeTable(ctx context.Context, src *sql.DB, spec mergeSpec) (int64, error) {
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

	_, _ = s.db.ExecContext(ctx, "DROP TABLE IF EXISTS temp."+stage)
	if _, err := s.db.ExecContext(ctx,
		"CREATE TEMP TABLE "+stage+" AS SELECT "+colList+" FROM main."+spec.table+" WHERE 0"); err != nil {
		return 0, err
	}
	defer func() { _, _ = s.db.ExecContext(context.WithoutCancel(ctx), "DROP TABLE IF EXISTS temp."+stage) }()

	if err := stageRows(ctx, src, spec.table, cols, s.db, stage); err != nil {
		return 0, err
	}

	on := make([]string, len(spec.key))
	for i, k := range spec.key {
		on[i] = "m." + k + " = " + stage + "." + k
	}
	q := "INSERT INTO " + spec.table + " (" + colList + ") SELECT " + colList +
		" FROM " + stage + " WHERE NOT EXISTS (SELECT 1 FROM " + spec.table + " m WHERE " +
		strings.Join(on, " AND ") + ")"

	res, err := s.db.ExecContext(ctx, q)
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

// stageRows copies every source row into the staging table in batches.
func stageRows(ctx context.Context, src *sql.DB, table string, cols []string, dst *sql.DB, stage string) error {
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

	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
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
// The copy goes to a sibling temp file and is renamed into place, so an
// interrupted restore never leaves a half-written store behind. Stale -wal/-shm
// files next to the destination are removed: they belong to the database being
// replaced, and leaving them in place is how a hand-copied store ends up corrupt.
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}

	tmp := dst + ".restore-tmp"
	if err := copyStoreFile(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install restore: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dst + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", suffix, err)
		}
	}
	return nil
}

func copyStoreFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restore temp: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy backup: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync restore: %w", err)
	}
	return out.Close()
}
