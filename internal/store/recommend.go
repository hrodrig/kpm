package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Recommendation is a persisted rightsizing snapshot (#9b).
type Recommendation struct {
	ID                int64
	RecordedAt        time.Time
	ClientID          string
	ClusterName       string
	Grain             string // workload | pod
	GrainKey          string
	Namespace         string
	Pod               string
	Container         string
	OwnerKind         string
	OwnerName         string
	Samples           int
	Percentile        int
	TargetUtilization float64
	// CPUTargetUtilization / MemoryTargetUtilization are the per-resource targets
	// the suggestion was computed with (#25). 0 means "inherit TargetUtilization",
	// which is also how every pre-#25 row reads, so old snapshots stay scoreable.
	CPUTargetUtilization    float64
	MemoryTargetUtilization float64
	WindowDays              int
	// DaysWithSamples is how much of WindowDays actually held data when the
	// snapshot was taken (#26). 0 means "unknown", the reading for rows written
	// before the column existed.
	DaysWithSamples int
	CPUPMcores      float64
	MemPBytes       int64
	CPUSugMcores    float64
	MemSugBytes     int64
	CPUReqMcores    float64
	MemReqBytes     int64
}

// CPUTarget resolves the CPU target recorded with this snapshot.
func (r Recommendation) CPUTarget() float64 {
	if r.CPUTargetUtilization > 0 {
		return r.CPUTargetUtilization
	}
	return r.TargetUtilization
}

// MemTarget resolves the memory target recorded with this snapshot.
func (r Recommendation) MemTarget() float64 {
	if r.MemoryTargetUtilization > 0 {
		return r.MemoryTargetUtilization
	}
	return r.TargetUtilization
}

// recommendationColumns is the read order shared by every SELECT and Scan.
const recommendationColumns = `
	recorded_at, client_id, cluster_name,
	grain, grain_key, namespace, pod, container, owner_kind, owner_name,
	samples, percentile, target_utilization,
	cpu_target_utilization, memory_target_utilization, window_days, days_with_samples,
	cpu_p_mcores, mem_p_bytes, cpu_sug_mcores, mem_sug_bytes,
	cpu_req_mcores, mem_req_bytes`

func migrateRecommendations(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS recommendations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			client_id TEXT NOT NULL DEFAULT '',
			cluster_name TEXT NOT NULL DEFAULT '',
			grain TEXT NOT NULL,
			grain_key TEXT NOT NULL,
			namespace TEXT NOT NULL DEFAULT '',
			pod TEXT NOT NULL DEFAULT '',
			container TEXT NOT NULL DEFAULT '',
			owner_kind TEXT NOT NULL DEFAULT '',
			owner_name TEXT NOT NULL DEFAULT '',
			samples INTEGER NOT NULL DEFAULT 0,
			percentile INTEGER NOT NULL DEFAULT 0,
			target_utilization REAL NOT NULL DEFAULT 0,
			cpu_target_utilization REAL NOT NULL DEFAULT 0,
			memory_target_utilization REAL NOT NULL DEFAULT 0,
			window_days INTEGER NOT NULL DEFAULT 0,
			days_with_samples INTEGER NOT NULL DEFAULT 0,
			cpu_p_mcores REAL NOT NULL DEFAULT 0,
			mem_p_bytes INTEGER NOT NULL DEFAULT 0,
			cpu_sug_mcores REAL NOT NULL DEFAULT 0,
			mem_sug_bytes INTEGER NOT NULL DEFAULT 0,
			cpu_req_mcores REAL NOT NULL DEFAULT 0,
			mem_req_bytes INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rec_grain_key_at
			ON recommendations(grain, grain_key, recorded_at)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("migrate recommendations: %w", err)
		}
	}
	// CREATE TABLE IF NOT EXISTS is a no-op on an existing database, so the
	// per-resource target columns need an explicit ALTER for DBs created before
	// #25. Default 0 keeps the old "inherit target_utilization" semantics.
	for _, col := range []string{"cpu_target_utilization", "memory_target_utilization"} {
		if err := addColumnIfMissing(db, "recommendations", col, "REAL NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	// Same for the coverage column added by #26; default 0 keeps the old
	// "coverage unknown" reading.
	if err := addColumnIfMissing(db, "recommendations", "days_with_samples", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

// addColumnIfMissing adds a column unless PRAGMA table_info already lists it.
func addColumnIfMissing(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    any
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("inspect %s: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect %s: %w", table, err)
	}
	if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddl)); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

// InsertRecommendations writes recommendation snapshots in one transaction.
func (s *Store) InsertRecommendations(ctx context.Context, rows []Recommendation) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO recommendations (
			recorded_at, client_id, cluster_name,
			grain, grain_key,
			namespace, pod, container, owner_kind, owner_name,
			samples, percentile, target_utilization,
			cpu_target_utilization, memory_target_utilization, window_days, days_with_samples,
			cpu_p_mcores, mem_p_bytes, cpu_sug_mcores, mem_sug_bytes,
			cpu_req_mcores, mem_req_bytes
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range rows {
		at := FormatUTC(r.RecordedAt)
		if _, err := stmt.ExecContext(ctx,
			at, r.ClientID, r.ClusterName,
			r.Grain, r.GrainKey,
			r.Namespace, r.Pod, r.Container, r.OwnerKind, r.OwnerName,
			r.Samples, r.Percentile, r.TargetUtilization,
			r.CPUTargetUtilization, r.MemoryTargetUtilization, r.WindowDays, r.DaysWithSamples,
			r.CPUPMcores, r.MemPBytes, r.CPUSugMcores, r.MemSugBytes,
			r.CPUReqMcores, r.MemReqBytes,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LatestRecommendation returns the newest snapshot for grain+key, or nil.
func (s *Store) LatestRecommendation(ctx context.Context, grain, grainKey string) (*Recommendation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, `+recommendationColumns+`
		FROM recommendations
		WHERE grain = ? AND grain_key = ?
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1`, grain, grainKey)
	rec, err := scanRecommendation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// LatestRecommendations returns the newest snapshot per grain_key (optional grain filter).
func (s *Store) LatestRecommendations(ctx context.Context, grain string) ([]Recommendation, error) {
	q := `
		SELECT r.id, r.recorded_at, r.client_id, r.cluster_name,
			r.grain, r.grain_key, r.namespace, r.pod, r.container, r.owner_kind, r.owner_name,
			r.samples, r.percentile, r.target_utilization,
			r.cpu_target_utilization, r.memory_target_utilization, r.window_days, r.days_with_samples,
			r.cpu_p_mcores, r.mem_p_bytes, r.cpu_sug_mcores, r.mem_sug_bytes,
			r.cpu_req_mcores, r.mem_req_bytes
		FROM recommendations r
		INNER JOIN (
			SELECT grain, grain_key, MAX(recorded_at) AS max_at
			FROM recommendations`
	args := []any{}
	if grain != "" {
		q += ` WHERE grain = ?`
		args = append(args, grain)
	}
	q += `
			GROUP BY grain, grain_key
		) latest ON r.grain = latest.grain AND r.grain_key = latest.grain_key AND r.recorded_at = latest.max_at
		ORDER BY r.grain, r.grain_key`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list latest recommendations: %w", err)
	}
	defer rows.Close()
	var out []Recommendation
	for rows.Next() {
		rec, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRecommendation(row scannable) (*Recommendation, error) {
	var r Recommendation
	var at string
	err := row.Scan(
		&r.ID, &at, &r.ClientID, &r.ClusterName,
		&r.Grain, &r.GrainKey, &r.Namespace, &r.Pod, &r.Container, &r.OwnerKind, &r.OwnerName,
		&r.Samples, &r.Percentile, &r.TargetUtilization,
		&r.CPUTargetUtilization, &r.MemoryTargetUtilization, &r.WindowDays, &r.DaysWithSamples,
		&r.CPUPMcores, &r.MemPBytes, &r.CPUSugMcores, &r.MemSugBytes,
		&r.CPUReqMcores, &r.MemReqBytes,
	)
	if err != nil {
		return nil, err
	}
	t, err := ParseUTC(at)
	if err != nil {
		return nil, fmt.Errorf("parse recorded_at %q: %w", at, err)
	}
	r.RecordedAt = t
	return &r, nil
}
