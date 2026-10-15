// Package mcp exposes read-only store tools over the Model Context Protocol.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

// DefaultListCap is the default hard row cap for list_samples.
const DefaultListCap = 500

// MaxListCap is the absolute maximum rows list_samples may return.
const MaxListCap = 2000

// Deps is the read-only tool backend (store + status meta).
type Deps struct {
	Store   *store.Store
	Config  *config.Config
	Version string
	Now     func() time.Time
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

// StatusOut is get_status structured output (no secrets).
type StatusOut struct {
	Version              string `json:"version"`
	StorePath            string `json:"store_path"`
	SampleIntervalSec    int    `json:"sample_interval_seconds"`
	NotificationsEnabled bool   `json:"notifications_enabled"`
	MCPEnabled           bool   `json:"mcp_enabled"`
	ClusterName          string `json:"cluster_name,omitempty"`
}

// GetStatus returns process/config meta for agents.
func (d *Deps) GetStatus(_ context.Context) (StatusOut, error) {
	path := ""
	if d.Store != nil {
		path = d.Store.Path()
	}
	cfg := d.Config
	out := StatusOut{Version: d.Version, StorePath: path}
	if cfg != nil {
		out.SampleIntervalSec = cfg.Sample.IntervalSeconds
		out.NotificationsEnabled = cfg.Notifications.Enabled
		out.MCPEnabled = cfg.MCP.Enabled
		out.ClusterName = cfg.Cluster.Name
	}
	return out, nil
}

// ListSamplesIn is list_samples input.
type ListSamplesIn struct {
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace filter"`
	Workload  string `json:"workload,omitempty" jsonschema:"Owner name / workload filter"`
	Since     string `json:"since,omitempty" jsonschema:"RFC3339 lower bound inclusive"`
	Until     string `json:"until,omitempty" jsonschema:"RFC3339 upper bound exclusive"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max rows; default 500, max 2000"`
}

// SampleRow is one anonymized sample row for tools.
type SampleRow struct {
	SampledAt        string  `json:"sampled_at"`
	Namespace        string  `json:"namespace"`
	Pod              string  `json:"pod"`
	Container        string  `json:"container"`
	OwnerKind        string  `json:"owner_kind,omitempty"`
	OwnerName        string  `json:"owner_name,omitempty"`
	CPUUtilMcores    float64 `json:"cpu_util_mcores"`
	MemUtilBytes     int64   `json:"mem_util_bytes"`
	CPURequestMcores float64 `json:"cpu_request_mcores,omitempty"`
	MemRequestBytes  int64   `json:"mem_request_bytes,omitempty"`
	CPUUtilRatio     float64 `json:"cpu_util_ratio,omitempty"`
	MemUtilRatio     float64 `json:"mem_util_ratio,omitempty"`
}

// ListSamplesOut is list_samples output.
type ListSamplesOut struct {
	Count     int         `json:"count"`
	Truncated bool        `json:"truncated"`
	Rows      []SampleRow `json:"rows"`
}

// ListSamples reads store samples with filters and a hard row cap.
// The SQL LIMIT is applied before materialization (newest rows + one probe).
func (d *Deps) ListSamples(ctx context.Context, in ListSamplesIn) (ListSamplesOut, error) {
	if d.Store == nil {
		return ListSamplesOut{}, fmt.Errorf("store not open")
	}
	f, err := filterFrom(in.Namespace, in.Workload, in.Since, in.Until)
	if err != nil {
		return ListSamplesOut{}, err
	}
	capN := in.Limit
	if capN <= 0 {
		capN = DefaultListCap
	}
	if capN > MaxListCap {
		capN = MaxListCap
	}
	f.Limit = capN + 1 // +1 probes truncation without loading the full window
	f.Newest = true
	recs, err := d.Store.ListRecords(ctx, f)
	if err != nil {
		return ListSamplesOut{}, err
	}
	out := ListSamplesOut{Truncated: len(recs) > capN}
	if len(recs) > capN {
		recs = recs[len(recs)-capN:]
	}
	out.Rows = make([]SampleRow, 0, len(recs))
	for _, r := range recs {
		out.Rows = append(out.Rows, toSampleRow(r))
	}
	out.Count = len(out.Rows)
	return out, nil
}

// SummarizeIn is summarize_window input.
type SummarizeIn struct {
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace filter"`
	Workload  string `json:"workload,omitempty" jsonschema:"Owner name filter"`
	Since     string `json:"since,omitempty" jsonschema:"RFC3339 lower bound"`
	Until     string `json:"until,omitempty" jsonschema:"RFC3339 upper bound"`
}

// SummarizeOut aggregates over matching samples (no Metrics API).
type SummarizeOut struct {
	SampleCount int     `json:"sample_count"`
	CPUUtilP95  float64 `json:"cpu_util_ratio_p95,omitempty"`
	MemUtilP95  float64 `json:"mem_util_ratio_p95,omitempty"`
	CPUUtilAvg  float64 `json:"cpu_util_ratio_avg,omitempty"`
	MemUtilAvg  float64 `json:"mem_util_ratio_avg,omitempty"`
	WithCPUReq  int     `json:"rows_with_cpu_request"`
	WithMemReq  int     `json:"rows_with_mem_request"`
}

// SummarizeWindow aggregates util ratios when request columns are present.
func (d *Deps) SummarizeWindow(ctx context.Context, in SummarizeIn) (SummarizeOut, error) {
	if d.Store == nil {
		return SummarizeOut{}, fmt.Errorf("store not open")
	}
	f, err := filterFrom(in.Namespace, in.Workload, in.Since, in.Until)
	if err != nil {
		return SummarizeOut{}, err
	}
	recs, err := d.Store.ListRecords(ctx, f)
	if err != nil {
		return SummarizeOut{}, err
	}
	out := SummarizeOut{SampleCount: len(recs)}
	var cpuRatios, memRatios []float64
	for _, r := range recs {
		if r.CPURequestMcores > 0 {
			out.WithCPUReq++
			cpuRatios = append(cpuRatios, float64(r.CPUUtilMcores)/float64(r.CPURequestMcores))
		}
		if r.MemRequestBytes > 0 {
			out.WithMemReq++
			memRatios = append(memRatios, float64(r.MemUtilBytes)/float64(r.MemRequestBytes))
		}
	}
	if len(cpuRatios) > 0 {
		out.CPUUtilAvg = mean(cpuRatios)
		out.CPUUtilP95 = percentile(cpuRatios, 95)
	}
	if len(memRatios) > 0 {
		out.MemUtilAvg = mean(memRatios)
		out.MemUtilP95 = percentile(memRatios, 95)
	}
	return out, nil
}

// ListRecsIn is list_recommendations input.
type ListRecsIn struct {
	Grain string `json:"grain,omitempty" jsonschema:"Grain filter: workload or pod"`
}

// RecRow is one recommendation snapshot row.
type RecRow struct {
	RecordedAt   string  `json:"recorded_at"`
	Grain        string  `json:"grain"`
	GrainKey     string  `json:"grain_key"`
	Namespace    string  `json:"namespace,omitempty"`
	OwnerKind    string  `json:"owner_kind,omitempty"`
	OwnerName    string  `json:"owner_name,omitempty"`
	Samples      int     `json:"samples"`
	CPUSugMcores float64 `json:"cpu_sug_mcores"`
	MemSugBytes  int64   `json:"mem_sug_bytes"`
	CPUReqMcores float64 `json:"cpu_req_mcores"`
	MemReqBytes  int64   `json:"mem_req_bytes"`
	TargetUtil   float64 `json:"target_utilization"`
}

// ListRecsOut is list_recommendations output.
type ListRecsOut struct {
	Count int      `json:"count"`
	Rows  []RecRow `json:"rows"`
}

// ListRecommendations returns latest recommendation snapshots.
func (d *Deps) ListRecommendations(ctx context.Context, in ListRecsIn) (ListRecsOut, error) {
	if d.Store == nil {
		return ListRecsOut{}, fmt.Errorf("store not open")
	}
	recs, err := d.Store.LatestRecommendations(ctx, in.Grain)
	if err != nil {
		return ListRecsOut{}, err
	}
	out := ListRecsOut{Rows: make([]RecRow, 0, len(recs))}
	for _, r := range recs {
		out.Rows = append(out.Rows, RecRow{
			RecordedAt:   r.RecordedAt.UTC().Format(time.RFC3339),
			Grain:        r.Grain,
			GrainKey:     r.GrainKey,
			Namespace:    r.Namespace,
			OwnerKind:    r.OwnerKind,
			OwnerName:    r.OwnerName,
			Samples:      r.Samples,
			CPUSugMcores: r.CPUSugMcores,
			MemSugBytes:  r.MemSugBytes,
			CPUReqMcores: r.CPUReqMcores,
			MemReqBytes:  r.MemReqBytes,
			TargetUtil:   r.TargetUtilization,
		})
	}
	out.Count = len(out.Rows)
	return out, nil
}

func filterFrom(ns, workload, since, until string) (store.ExportFilter, error) {
	f := store.ExportFilter{Namespace: ns, Workload: workload}
	var err error
	if since != "" {
		f.Since, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return f, fmt.Errorf("since: %w", err)
		}
	}
	if until != "" {
		f.Until, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return f, fmt.Errorf("until: %w", err)
		}
	}
	return f, nil
}

func toSampleRow(r store.Record) SampleRow {
	row := SampleRow{
		SampledAt:        r.SampledAt.UTC().Format(time.RFC3339),
		Namespace:        r.Namespace,
		Pod:              r.Pod,
		Container:        r.Container,
		OwnerKind:        r.OwnerKind,
		OwnerName:        r.OwnerName,
		CPUUtilMcores:    r.CPUUtilMcores,
		MemUtilBytes:     r.MemUtilBytes,
		CPURequestMcores: r.CPURequestMcores,
		MemRequestBytes:  r.MemRequestBytes,
	}
	if r.CPURequestMcores > 0 {
		row.CPUUtilRatio = float64(r.CPUUtilMcores) / float64(r.CPURequestMcores)
	}
	if r.MemRequestBytes > 0 {
		row.MemUtilRatio = float64(r.MemUtilBytes) / float64(r.MemRequestBytes)
	}
	return row
}

func mean(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func percentile(xs []float64, p int) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]float64(nil), xs...)
	sort.Float64s(cp)
	if p <= 0 {
		return cp[0]
	}
	if p >= 100 {
		return cp[len(cp)-1]
	}
	rank := (float64(p) / 100) * float64(len(cp)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return cp[lo]
	}
	w := rank - float64(lo)
	return cp[lo]*(1-w) + cp[hi]*w
}

// MustJSON is a test helper encoding value as compact JSON text.
func MustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
