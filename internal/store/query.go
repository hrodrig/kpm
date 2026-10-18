package store

import (
	"context"
	"fmt"
	"strings"
)

// ListRecords returns samples matching the filter (same selectors as Export).
// Results are always in ascending sampled_at order. When Newest is set with
// Limit, the newest Limit rows are selected then reversed to ascending.
func (s *Store) ListRecords(ctx context.Context, f ExportFilter) ([]Record, error) {
	q, args := listRecordsSQL(f)
	rows, err := s.reads().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list samples: %w", err)
	}
	defer rows.Close()

	out, err := scanRecords(rows)
	if err != nil {
		return nil, err
	}
	if f.Newest && f.Limit > 0 {
		reverseRecords(out)
	}
	return out, nil
}

func listRecordsSQL(f ExportFilter) (string, []any) {
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
		args = append(args, FormatUTC(f.Since))
	}
	if !f.Until.IsZero() {
		q += ` AND sampled_at < ?`
		args = append(args, FormatUTC(f.Until))
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
	if f.Newest && f.Limit > 0 {
		q += ` ORDER BY sampled_at DESC, id DESC`
	} else {
		q += ` ORDER BY sampled_at ASC, id ASC`
	}
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	return q, args
}

func scanRecords(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]Record, error) {
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
		t, err := ParseUTC(at)
		if err != nil {
			return nil, fmt.Errorf("parse sampled_at %q: %w", at, err)
		}
		r.SampledAt = t
		out = append(out, r)
	}
	return out, rows.Err()
}

func reverseRecords(out []Record) {
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
}
