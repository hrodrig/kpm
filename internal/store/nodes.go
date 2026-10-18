package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// NodeSnapshot is one persisted node capacity row (#20b).
type NodeSnapshot struct {
	ID             int64
	RecordedAt     time.Time
	ClientID       string
	ClusterName    string
	Node           string
	CPUAllocMcores float64
	MemAllocBytes  int64
	CPUCapMcores   float64
	MemCapBytes    int64
	CPUReqMcores   float64
	MemReqBytes    int64
	CPUUtilMcores  float64
	MemUtilBytes   int64
	Pods           int

	// Pool identity, as of schema v7. Taints holds only the blocking effects
	// (NoSchedule, NoExecute) in "key=value:Effect" form; stored comma-joined
	// because a taint key cannot contain a comma.
	Taints        []string
	Unschedulable bool
}

func migrateNodeSnapshots(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS node_snapshots (
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
			pods INTEGER NOT NULL DEFAULT 0,
			taints TEXT NOT NULL DEFAULT '',
			unschedulable INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_node_snap_at ON node_snapshots(recorded_at)`,
		`CREATE INDEX IF NOT EXISTS idx_node_snap_node_at ON node_snapshots(node, recorded_at)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("migrate node_snapshots: %w", err)
		}
	}
	// Stores written before v7 have the table without pool identity.
	if err := addColumnIfMissing(db, "node_snapshots", "taints", `TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return addColumnIfMissing(db, "node_snapshots", "unschedulable", `INTEGER NOT NULL DEFAULT 0`)
}

// joinTaints and splitTaints carry pool identity through a TEXT column. The
// canonical form is produced by live.BlockingTaints; keys and values cannot
// contain a comma, so it is a safe separator.
func joinTaints(taints []string) string {
	return strings.Join(taints, ",")
}

func splitTaints(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// InsertNodeSnapshots writes a capture batch in one transaction.
func (s *Store) InsertNodeSnapshots(ctx context.Context, rows []NodeSnapshot) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO node_snapshots (
			recorded_at, client_id, cluster_name, node,
			cpu_alloc_mcores, mem_alloc_bytes, cpu_cap_mcores, mem_cap_bytes,
			cpu_req_mcores, mem_req_bytes, cpu_util_mcores, mem_util_bytes, pods,
			taints, unschedulable
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx,
			FormatUTC(r.RecordedAt), r.ClientID, r.ClusterName, r.Node,
			r.CPUAllocMcores, r.MemAllocBytes, r.CPUCapMcores, r.MemCapBytes,
			r.CPUReqMcores, r.MemReqBytes, r.CPUUtilMcores, r.MemUtilBytes, r.Pods,
			joinTaints(r.Taints), boolToInt(r.Unschedulable),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// LatestNodeSnapshots returns all rows from the newest recorded_at batch.
func (s *Store) LatestNodeSnapshots(ctx context.Context) ([]NodeSnapshot, error) {
	var at string
	err := s.reads().QueryRowContext(ctx, `SELECT MAX(recorded_at) FROM node_snapshots`).Scan(&at)
	if err == sql.ErrNoRows || at == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.reads().QueryContext(ctx, `
		SELECT id, recorded_at, client_id, cluster_name, node,
			cpu_alloc_mcores, mem_alloc_bytes, cpu_cap_mcores, mem_cap_bytes,
			cpu_req_mcores, mem_req_bytes, cpu_util_mcores, mem_util_bytes, pods,
			taints, unschedulable
		FROM node_snapshots
		WHERE recorded_at = ?
		ORDER BY node ASC`, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []NodeSnapshot
	for rows.Next() {
		var r NodeSnapshot
		var ats, taints string
		var unsched int
		if err := rows.Scan(
			&r.ID, &ats, &r.ClientID, &r.ClusterName, &r.Node,
			&r.CPUAllocMcores, &r.MemAllocBytes, &r.CPUCapMcores, &r.MemCapBytes,
			&r.CPUReqMcores, &r.MemReqBytes, &r.CPUUtilMcores, &r.MemUtilBytes, &r.Pods,
			&taints, &unsched,
		); err != nil {
			return nil, err
		}
		r.Taints = splitTaints(taints)
		r.Unschedulable = unsched != 0
		t, err := ParseUTC(ats)
		if err != nil {
			return nil, err
		}
		r.RecordedAt = t
		out = append(out, r)
	}
	return out, rows.Err()
}
