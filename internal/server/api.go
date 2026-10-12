// Package server provides the read-only JSON API (/api/v1/*) for analyze-local.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
	"github.com/hrodrig/kpm/internal/recommend"
	"github.com/hrodrig/kpm/internal/store"
)

// APIPrefix is the JSON API root path.
const APIPrefix = "/api/v1"

// DefaultMaxSeriesPoints caps series response length before downsample.
const DefaultMaxSeriesPoints = 300

// Deps is the read-only API backend.
type Deps struct {
	Store   *store.Store
	Config  *config.Config
	Version string
	Now     func() time.Time
	// Live is optional; nil → GET /live returns 503 JSON.
	Live func(ctx context.Context) (LiveOut, error)
}

func (d *Deps) now() time.Time {
	if d != nil && d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

// Mount registers /api/v1/* on mux, optionally behind bearer auth.
func Mount(mux *http.ServeMux, d *Deps, bearerToken string) {
	api := &api{deps: d}
	wrap := func(h http.HandlerFunc) http.Handler {
		return httpserver.BearerAuth(bearerToken, h)
	}
	mux.Handle("GET "+APIPrefix+"/status", wrap(api.handleStatus))
	mux.Handle("GET "+APIPrefix+"/workloads", wrap(api.handleWorkloads))
	mux.Handle("GET "+APIPrefix+"/series", wrap(api.handleSeries))
	mux.Handle("GET "+APIPrefix+"/recommendations", wrap(api.handleRecommendations))
	mux.Handle("GET "+APIPrefix+"/live", wrap(api.handleLive))
	mux.Handle("GET "+APIPrefix+"/export/patch", wrap(api.handleExportPatch))
}

// ResolveAPIToken picks server.token_env, else mcp.token_env when sharing.
func ResolveAPIToken(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", nil
	}
	if te := strings.TrimSpace(cfg.Server.TokenEnv); te != "" {
		return httpserver.ResolveTokenEnv(te)
	}
	if te := strings.TrimSpace(cfg.MCP.TokenEnv); te != "" {
		return httpserver.ResolveTokenEnv(te)
	}
	return "", nil
}

type api struct {
	deps *Deps
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// StatusOut is GET /api/v1/status (no secrets).
type StatusOut struct {
	Version              string `json:"version"`
	StorePath            string `json:"store_path"`
	SampleIntervalSec    int    `json:"sample_interval_seconds"`
	NotificationsEnabled bool   `json:"notifications_enabled"`
	MCPEnabled           bool   `json:"mcp_enabled"`
	ServerEnabled        bool   `json:"server_enabled"`
	ClusterName          string `json:"cluster_name,omitempty"`
	ClusterEnvironment   string `json:"cluster_environment,omitempty"`
	KubeContext          string `json:"kube_context,omitempty"`
	KubeLive             bool   `json:"kube_live"` // true when /api/v1/live client is wired
	SampleCount          int64  `json:"sample_count,omitempty"`
}

func (a *api) handleStatus(w http.ResponseWriter, r *http.Request) {
	d := a.deps
	out := StatusOut{Version: d.Version, KubeLive: d.Live != nil}
	if d.Store != nil {
		out.StorePath = d.Store.Path()
		if n, err := d.Store.Count(r.Context()); err == nil {
			out.SampleCount = n
		}
	}
	if cfg := d.Config; cfg != nil {
		out.SampleIntervalSec = cfg.Sample.IntervalSeconds
		out.NotificationsEnabled = cfg.Notifications.Enabled
		out.MCPEnabled = cfg.MCP.Enabled
		out.ServerEnabled = cfg.Server.Enabled
		out.ClusterName = strings.TrimSpace(cfg.Cluster.Name)
		out.ClusterEnvironment = strings.TrimSpace(cfg.Cluster.Environment)
		out.KubeContext = strings.TrimSpace(cfg.Kube.Context)
	}
	writeJSON(w, http.StatusOK, out)
}

// WorkloadOut is one grain with activity in the window.
type WorkloadOut struct {
	Namespace string `json:"namespace"`
	OwnerKind string `json:"owner_kind,omitempty"`
	OwnerName string `json:"owner_name,omitempty"`
	GrainKey  string `json:"grain_key"`
	Samples   int64  `json:"samples"`
	FirstAt   string `json:"first_at,omitempty"`
	LastAt    string `json:"last_at,omitempty"`
}

// WorkloadsOut is GET /api/v1/workloads.
type WorkloadsOut struct {
	Count int           `json:"count"`
	Items []WorkloadOut `json:"items"`
}

func (a *api) handleWorkloads(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeErr(w, http.StatusServiceUnavailable, "store not open")
		return
	}
	f, err := filterFromQuery(r, a.deps.now())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := listWorkloads(r.Context(), a.deps.Store, f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, WorkloadsOut{Count: len(items), Items: items})
}

func listWorkloads(ctx context.Context, st *store.Store, f store.ExportFilter) ([]WorkloadOut, error) {
	recs, err := st.ListRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	type agg struct {
		ns, kind, name string
		n              int64
		first, last    time.Time
	}
	m := map[string]*agg{}
	for _, r := range recs {
		key := grainKey(r.Namespace, r.OwnerKind, r.OwnerName, r.Pod)
		a := m[key]
		if a == nil {
			a = &agg{ns: r.Namespace, kind: r.OwnerKind, name: r.OwnerName, first: r.SampledAt, last: r.SampledAt}
			if a.name == "" {
				a.name = r.Pod
			}
			m[key] = a
		}
		a.n++
		if r.SampledAt.Before(a.first) {
			a.first = r.SampledAt
		}
		if r.SampledAt.After(a.last) {
			a.last = r.SampledAt
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]WorkloadOut, 0, len(keys))
	for _, k := range keys {
		a := m[k]
		out = append(out, WorkloadOut{
			Namespace: a.ns,
			OwnerKind: a.kind,
			OwnerName: a.name,
			GrainKey:  k,
			Samples:   a.n,
			FirstAt:   a.first.UTC().Format(time.RFC3339),
			LastAt:    a.last.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func grainKey(ns, kind, name, pod string) string {
	if strings.TrimSpace(name) != "" {
		if kind == "" {
			kind = "Workload"
		}
		return ns + "/" + kind + "/" + name
	}
	return ns + "/Pod/" + pod
}

// SeriesPoint is one downsampled metric sample.
type SeriesPoint struct {
	T string  `json:"t"`
	V float64 `json:"v"`
}

// SeriesOut is GET /api/v1/series.
type SeriesOut struct {
	Metric      string        `json:"metric"`
	Namespace   string        `json:"namespace,omitempty"`
	Workload    string        `json:"workload,omitempty"`
	Count       int           `json:"count"`
	Downsampled bool          `json:"downsampled"`
	Points      []SeriesPoint `json:"points"`
	// Request / Limit are reference levels in the same units as Points
	// (absolute mcores/bytes, or ratio of request for *_ratio metrics).
	Request *float64 `json:"request,omitempty"`
	Limit   *float64 `json:"limit,omitempty"`
}

func (a *api) handleSeries(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeErr(w, http.StatusServiceUnavailable, "store not open")
		return
	}
	q := r.URL.Query()
	metric := strings.TrimSpace(q.Get("metric"))
	if metric == "" {
		metric = "cpu_util_mcores"
	}
	f, err := filterFromQuery(r, a.deps.now())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	maxPts := DefaultMaxSeriesPoints
	if s := strings.TrimSpace(q.Get("max_points")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 5000 {
			writeErr(w, http.StatusBadRequest, "max_points must be 1..5000")
			return
		}
		maxPts = n
	}
	recs, err := a.deps.Store.ListRecords(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	raw := make([]SeriesPoint, 0, len(recs))
	for _, rec := range recs {
		v, ok := metricValue(rec, metric)
		if !ok {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("unknown metric %q", metric))
			return
		}
		raw = append(raw, SeriesPoint{T: rec.SampledAt.UTC().Format(time.RFC3339), V: v})
	}
	pts, down := downsample(raw, maxPts)
	req, lim := seriesRefs(recs, metric)
	writeJSON(w, http.StatusOK, SeriesOut{
		Metric:      metric,
		Namespace:   f.Namespace,
		Workload:    f.Workload,
		Count:       len(pts),
		Downsampled: down,
		Points:      pts,
		Request:     req,
		Limit:       lim,
	})
}

// seriesRefs returns request/limit guides from the newest record that has them.
func seriesRefs(recs []store.Record, metric string) (req, lim *float64) {
	for i := len(recs) - 1; i >= 0; i-- {
		req, lim = recordRefs(recs[i], metric)
		if req != nil || lim != nil {
			return req, lim
		}
	}
	return nil, nil
}

func recordRefs(r store.Record, metric string) (req, lim *float64) {
	switch metric {
	case "cpu_util_mcores":
		return floatPtrIf(r.CPURequestMcores > 0, r.CPURequestMcores),
			floatPtrIf(r.CPULimitMcores > 0, r.CPULimitMcores)
	case "mem_util_bytes":
		return floatPtrIf(r.MemRequestBytes > 0, float64(r.MemRequestBytes)),
			floatPtrIf(r.MemLimitBytes > 0, float64(r.MemLimitBytes))
	case "cpu_util_ratio":
		if r.CPURequestMcores <= 0 {
			return nil, nil
		}
		return floatPtr(1), floatPtrIf(r.CPULimitMcores > 0, r.CPULimitMcores/r.CPURequestMcores)
	case "mem_util_ratio":
		if r.MemRequestBytes <= 0 {
			return nil, nil
		}
		return floatPtr(1), floatPtrIf(r.MemLimitBytes > 0,
			float64(r.MemLimitBytes)/float64(r.MemRequestBytes))
	default:
		return nil, nil
	}
}

func floatPtr(v float64) *float64 { return &v }

func floatPtrIf(ok bool, v float64) *float64 {
	if !ok {
		return nil
	}
	return &v
}

func metricValue(r store.Record, metric string) (float64, bool) {
	switch metric {
	case "cpu_util_mcores":
		return r.CPUUtilMcores, true
	case "mem_util_bytes":
		return float64(r.MemUtilBytes), true
	case "cpu_util_ratio":
		if r.CPURequestMcores <= 0 {
			return 0, true
		}
		return float64(r.CPUUtilMcores) / float64(r.CPURequestMcores), true
	case "mem_util_ratio":
		if r.MemRequestBytes <= 0 {
			return 0, true
		}
		return float64(r.MemUtilBytes) / float64(r.MemRequestBytes), true
	default:
		return 0, false
	}
}

func downsample(pts []SeriesPoint, max int) ([]SeriesPoint, bool) {
	if max < 1 || len(pts) <= max {
		return pts, false
	}
	out := make([]SeriesPoint, 0, max)
	n := len(pts)
	for i := 0; i < max; i++ {
		lo := i * n / max
		hi := (i + 1) * n / max
		if hi <= lo {
			hi = lo + 1
		}
		var sum float64
		for j := lo; j < hi && j < n; j++ {
			sum += pts[j].V
		}
		cnt := hi - lo
		if cnt < 1 {
			cnt = 1
		}
		out = append(out, SeriesPoint{T: pts[lo].T, V: sum / float64(cnt)})
	}
	return out, true
}

// RecOut is one recommendation row.
type RecOut struct {
	RecordedAt   string  `json:"recorded_at"`
	Grain        string  `json:"grain"`
	GrainKey     string  `json:"grain_key"`
	Namespace    string  `json:"namespace,omitempty"`
	OwnerKind    string  `json:"owner_kind,omitempty"`
	OwnerName    string  `json:"owner_name,omitempty"`
	Samples      int     `json:"samples"`
	CPUPMcores   float64 `json:"cpu_p_mcores"`
	MemPBytes    int64   `json:"mem_p_bytes"`
	CPUSugMcores float64 `json:"cpu_sug_mcores"`
	MemSugBytes  int64   `json:"mem_sug_bytes"`
	CPUReqMcores float64 `json:"cpu_req_mcores"`
	MemReqBytes  int64   `json:"mem_req_bytes"`
	CPUDeltaM    float64 `json:"cpu_delta_mcores"` // sug − req (negative ⇒ can shrink)
	MemDeltaB    int64   `json:"mem_delta_bytes"`
	CPUVerdict   string  `json:"cpu_verdict"` // oversized|undersized|ok|n/a
	MemVerdict   string  `json:"mem_verdict"`
	TargetUtil   float64 `json:"target_utilization"`
}

// RecsOut is GET /api/v1/recommendations.
type RecsOut struct {
	Count int      `json:"count"`
	Rows  []RecOut `json:"rows"`
}

func (a *api) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeErr(w, http.StatusServiceUnavailable, "store not open")
		return
	}
	grain := strings.TrimSpace(r.URL.Query().Get("grain"))
	recs, err := a.deps.Store.LatestRecommendations(r.Context(), grain)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	over, under := recommend.DefaultOversizeRatio, recommend.DefaultUndersizeRatio
	if cfg := a.deps.Config; cfg != nil {
		if cfg.Recommend.CPUOversizeRatio > 0 {
			over = cfg.Recommend.CPUOversizeRatio
		}
		if cfg.Recommend.CPUUndersizeRatio > 0 {
			under = cfg.Recommend.CPUUndersizeRatio
		}
	}
	out := RecsOut{Rows: make([]RecOut, 0, len(recs))}
	for _, rec := range recs {
		cpuV, memV := recommend.SnapshotVerdicts(rec.CPUPMcores, rec.CPUReqMcores, rec.MemPBytes, rec.MemReqBytes, over, under)
		out.Rows = append(out.Rows, RecOut{
			RecordedAt:   rec.RecordedAt.UTC().Format(time.RFC3339),
			Grain:        rec.Grain,
			GrainKey:     rec.GrainKey,
			Namespace:    rec.Namespace,
			OwnerKind:    rec.OwnerKind,
			OwnerName:    rec.OwnerName,
			Samples:      rec.Samples,
			CPUPMcores:   rec.CPUPMcores,
			MemPBytes:    rec.MemPBytes,
			CPUSugMcores: rec.CPUSugMcores,
			MemSugBytes:  rec.MemSugBytes,
			CPUReqMcores: rec.CPUReqMcores,
			MemReqBytes:  rec.MemReqBytes,
			CPUDeltaM:    rec.CPUSugMcores - rec.CPUReqMcores,
			MemDeltaB:    rec.MemSugBytes - rec.MemReqBytes,
			CPUVerdict:   string(cpuV),
			MemVerdict:   string(memV),
			TargetUtil:   rec.TargetUtilization,
		})
	}
	out.Count = len(out.Rows)
	writeJSON(w, http.StatusOK, out)
}

// LiveRow is one live container snapshot row.
type LiveRow struct {
	Namespace        string  `json:"namespace"`
	Pod              string  `json:"pod"`
	Container        string  `json:"container"`
	OwnerKind        string  `json:"owner_kind,omitempty"`
	OwnerName        string  `json:"owner_name,omitempty"`
	CPUUtilMcores    float64 `json:"cpu_util_mcores"`
	CPURequestMcores float64 `json:"cpu_request_mcores"`
	CPULimitMcores   float64 `json:"cpu_limit_mcores"`
	MemUtilBytes     int64   `json:"mem_util_bytes"`
	MemRequestBytes  int64   `json:"mem_request_bytes"`
	MemLimitBytes    int64   `json:"mem_limit_bytes"`
	HasUtil          bool    `json:"has_util"`
}

// LiveOut is GET /api/v1/live.
type LiveOut struct {
	Available  bool      `json:"available"`
	CapturedAt string    `json:"captured_at,omitempty"`
	Error      string    `json:"error,omitempty"`
	Count      int       `json:"count"`
	Rows       []LiveRow `json:"rows,omitempty"`
}

func (a *api) handleLive(w http.ResponseWriter, r *http.Request) {
	if a.deps.Live == nil {
		writeJSON(w, http.StatusServiceUnavailable, LiveOut{
			Available: false,
			Error:     "kube unavailable",
			Count:     0,
		})
		return
	}
	out, err := a.deps.Live(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, LiveOut{
			Available: false,
			Error:     err.Error(),
			Count:     0,
		})
		return
	}
	out.Available = true
	writeJSON(w, http.StatusOK, out)
}

// PatchExportOut is GET /api/v1/export/patch (GitOps copy/download; never applied).
type PatchExportOut struct {
	Format   string `json:"format"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
	GrainKey string `json:"grain_key"`
}

func (a *api) handleExportPatch(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeErr(w, http.StatusServiceUnavailable, "store not open")
		return
	}
	q := r.URL.Query()
	grainKey := strings.TrimSpace(q.Get("grain_key"))
	if grainKey == "" {
		writeErr(w, http.StatusBadRequest, "grain_key required")
		return
	}
	format := recommend.PatchFormat(strings.TrimSpace(q.Get("format")))
	if format == "" {
		format = recommend.PatchStrategic
	}
	recs, err := a.deps.Store.LatestRecommendations(r.Context(), "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var found *store.Recommendation
	for i := range recs {
		if recs[i].GrainKey == grainKey {
			found = &recs[i]
			break
		}
	}
	if found == nil {
		writeErr(w, http.StatusNotFound, "no recommendation for grain_key")
		return
	}
	exp, err := recommend.FormatResourcePatch(*found, format)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, PatchExportOut{
		Format:   string(exp.Format),
		Filename: exp.Filename,
		Content:  exp.Content,
		GrainKey: exp.GrainKey,
	})
}

func filterFromQuery(r *http.Request, now time.Time) (store.ExportFilter, error) {
	q := r.URL.Query()
	f := store.ExportFilter{
		Namespace: strings.TrimSpace(q.Get("namespace")),
		Workload:  strings.TrimSpace(q.Get("workload")),
		Pod:       strings.TrimSpace(q.Get("pod")),
	}
	since := strings.TrimSpace(q.Get("since"))
	until := strings.TrimSpace(q.Get("until"))
	window := strings.TrimSpace(q.Get("window"))
	if since == "" && until == "" && window != "" {
		d, err := parseWindow(window)
		if err != nil {
			return f, err
		}
		f.Since = now.Add(-d)
		f.Until = now
		return f, nil
	}
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

func parseWindow(w string) (time.Duration, error) {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "1h":
		return time.Hour, nil
	case "24h", "1d":
		return 24 * time.Hour, nil
	case "48h", "2d":
		return 48 * time.Hour, nil
	case "7d":
		return 7 * 24 * time.Hour, nil
	case "15d":
		return 15 * 24 * time.Hour, nil
	default:
		d, err := time.ParseDuration(w)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("window: want 1h|24h|48h|7d|15d or Go duration")
		}
		return d, nil
	}
}
