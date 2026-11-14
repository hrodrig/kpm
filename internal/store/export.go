package store

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"
)

// ExportFilter selects rows for export / ListRecords.
type ExportFilter struct {
	Since     time.Time // inclusive; zero = no lower bound
	Until     time.Time // exclusive; zero = no upper bound
	Namespace string
	Pod       string
	Workload  string // matches owner_name
	Container string // matches container name
	// Limit caps rows returned (0 = unlimited). When Newest is true, the
	// newest Limit rows are chosen (SQL DESC) then returned in ascending order.
	Limit  int
	Newest bool
}

// ExportFormat is csv or tsv.
type ExportFormat string

const (
	FormatCSV ExportFormat = "csv"
	FormatTSV ExportFormat = "tsv"
)

var exportHeader = []string{
	"sampled_at", "client_id", "cluster_name",
	"namespace", "pod", "container",
	"owner_kind", "owner_name", "node",
	"cpu_util_mcores", "mem_util_bytes",
	"cpu_request_mcores", "mem_request_bytes",
	"cpu_limit_mcores", "mem_limit_bytes",
}

func exportSep(format ExportFormat) (rune, error) {
	switch format {
	case FormatCSV:
		return ',', nil
	case FormatTSV:
		return '\t', nil
	default:
		return 0, fmt.Errorf("export format must be csv or tsv (got %q)", format)
	}
}

// appendSampleFilters adds ExportFilter predicates to a samples WHERE clause.
func appendSampleFilters(q string, args []any, f ExportFilter) (string, []any) {
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
	if c := strings.TrimSpace(f.Container); c != "" {
		q += ` AND container = ?`
		args = append(args, c)
	}
	return q, args
}

func writeExportRow(cw *csv.Writer, sampledAt, clientID, clusterName, namespace, pod, container, ownerKind, ownerName, node string, cpuUtil, cpuReq, cpuLim float64, memUtil, memReq, memLim int64) error {
	return cw.Write([]string{
		sampledAt, clientID, clusterName,
		namespace, pod, container,
		ownerKind, ownerName, node,
		fmt.Sprintf("%g", cpuUtil), fmt.Sprintf("%d", memUtil),
		fmt.Sprintf("%g", cpuReq), fmt.Sprintf("%d", memReq),
		fmt.Sprintf("%g", cpuLim), fmt.Sprintf("%d", memLim),
	})
}

// Export writes matching samples to w as csv or tsv. Returns row count.
func (s *Store) Export(ctx context.Context, w io.Writer, format ExportFormat, f ExportFilter) (int64, error) {
	sep, err := exportSep(format)
	if err != nil {
		return 0, err
	}

	q := `SELECT sampled_at, client_id, cluster_name,
		namespace, pod, container,
		owner_kind, owner_name, node,
		cpu_util_mcores, mem_util_bytes,
		cpu_request_mcores, mem_request_bytes,
		cpu_limit_mcores, mem_limit_bytes
		FROM samples WHERE 1=1`
	q, args := appendSampleFilters(q, nil, f)
	q += ` ORDER BY sampled_at ASC, id ASC`

	rows, err := s.reads().QueryContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	cw.Comma = sep
	if err := cw.Write(exportHeader); err != nil {
		return 0, err
	}

	var n int64
	for rows.Next() {
		var (
			sampledAt, clientID, clusterName string
			namespace, pod, container        string
			ownerKind, ownerName, node       string
			cpuUtil, cpuReq, cpuLim          float64
			memUtil, memReq, memLim          int64
		)
		if err := rows.Scan(
			&sampledAt, &clientID, &clusterName,
			&namespace, &pod, &container,
			&ownerKind, &ownerName, &node,
			&cpuUtil, &memUtil, &cpuReq, &memReq, &cpuLim, &memLim,
		); err != nil {
			return n, err
		}
		if err := writeExportRow(cw, sampledAt, clientID, clusterName, namespace, pod, container, ownerKind, ownerName, node, cpuUtil, cpuReq, cpuLim, memUtil, memReq, memLim); err != nil {
			return n, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	cw.Flush()
	return n, cw.Error()
}
