package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ListRecords returns samples matching the filter (same selectors as Export).
func (s *Store) ListRecords(ctx context.Context, f ExportFilter) ([]Record, error) {
	q := `SELECT sampled_at, client_id, cluster_name,
		namespace, pod, container,
		owner_kind, owner_name, node,
		cpu_util_mcores, mem_util_bytes,
		cpu_request_mcores, mem_request_bytes,
		cpu_limit_mcores, mem_limit_bytes
		FROM samples WHERE 1=1`
	var args []any
	if !f.Since.IsZero() {
		q += ` AND sampled_at >= ?`
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		q += ` AND sampled_at < ?`
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if ns := strings.TrimSpace(f.Namespace); ns != "" {
		q += ` AND namespace = ?`
		args = append(args, ns)
	}
	if pod := strings.TrimSpace(f.Pod); pod != "" {
		q += ` AND pod = ?`
		args = append(args, pod)
	}
	if wl := strings.TrimSpace(f.Workload); wl != "" {
		q += ` AND owner_name = ?`
		args = append(args, wl)
	}
	q += ` ORDER BY sampled_at ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list samples: %w", err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		var at string
		if err := rows.Scan(
			&at, &r.ClientID, &r.ClusterName,
			&r.Namespace, &r.Pod, &r.Container,
			&r.OwnerKind, &r.OwnerName, &r.Node,
			&r.CPUUtilMcores, &r.MemUtilBytes,
			&r.CPURequestMcores, &r.MemRequestBytes,
			&r.CPULimitMcores, &r.MemLimitBytes,
		); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			t, err = time.Parse(time.RFC3339, at)
			if err != nil {
				return nil, fmt.Errorf("parse sampled_at %q: %w", at, err)
			}
		}
		r.SampledAt = t.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
