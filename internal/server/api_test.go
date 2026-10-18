package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/server"
	"github.com/hrodrig/kpm/internal/store"
)

func fixtureStore(t *testing.T, now time.Time) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	recs := []store.Record{
		{
			SampledAt: now.Add(-2 * time.Hour), ClientID: "c", ClusterName: "x",
			Namespace: "app", Pod: "api-0", Container: "main",
			OwnerKind: "StatefulSet", OwnerName: "api",
			CPUUtilMcores: 100, MemUtilBytes: 1000, CPURequestMcores: 200, MemRequestBytes: 2000,
		},
		{
			SampledAt: now.Add(-1 * time.Hour), ClientID: "c", ClusterName: "x",
			Namespace: "app", Pod: "api-0", Container: "main",
			OwnerKind: "StatefulSet", OwnerName: "api",
			CPUUtilMcores: 150, MemUtilBytes: 1500, CPURequestMcores: 200, MemRequestBytes: 2000,
		},
	}
	if err := st.InsertRecords(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	return st
}

func authedGet(h http.Handler, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestAPIUnauthorized(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{Store: fixtureStore(t, now), Version: "v"}, "secret")
	rr := authedGet(mux, "/api/v1/status", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestAPIStatus(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	cfg := &config.Config{}
	cfg.Server.Enabled = true
	cfg.Cluster.Name = "demo-cluster"
	cfg.Cluster.Environment = "dev"
	cfg.Kube.Context = "demo-ctx"
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{
		Store: fixtureStore(t, now), Config: cfg, Version: "v-test",
		Now: func() time.Time { return now },
	}, "secret")

	rr := authedGet(mux, "/api/v1/status", "secret")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var stOut server.StatusOut
	if err := json.Unmarshal(rr.Body.Bytes(), &stOut); err != nil {
		t.Fatal(err)
	}
	if stOut.Version != "v-test" || stOut.SampleCount != 2 || !stOut.ServerEnabled {
		t.Fatalf("%+v", stOut)
	}
	if stOut.ClusterName != "demo-cluster" || stOut.ClusterEnvironment != "dev" || stOut.KubeContext != "demo-ctx" {
		t.Fatalf("cluster fields %+v", stOut)
	}
	if stOut.KubeLive {
		t.Fatal("kube_live want false without Live fn")
	}
}

func TestAPISeriesRefs(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{
		Store: fixtureStore(t, now), Version: "v-test",
		Now: func() time.Time { return now },
	}, "secret")
	rr := authedGet(mux, "/api/v1/series/batch?workloads=app/api&metric=cpu_util_mcores&window=24h", "secret")
	if rr.Code != http.StatusOK {
		t.Fatalf("batch %d %s", rr.Code, rr.Body.String())
	}
	var batch server.SeriesBatchOut
	if err := json.Unmarshal(rr.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Items) != 1 || batch.Items[0].Workload != "api" || batch.Items[0].Count < 1 {
		t.Fatalf("batch %+v", batch)
	}

	rr = authedGet(mux, "/api/v1/series?namespace=app&workload=api&metric=cpu_util_mcores&window=24h", "secret")
	if rr.Code != http.StatusOK {
		t.Fatalf("series %d %s", rr.Code, rr.Body.String())
	}
	var series server.SeriesOut
	if err := json.Unmarshal(rr.Body.Bytes(), &series); err != nil {
		t.Fatal(err)
	}
	if series.Count != 2 {
		t.Fatalf("series count %d", series.Count)
	}
	if series.Request == nil || *series.Request != 200 {
		t.Fatalf("request %+v", series.Request)
	}
	if series.Limit != nil {
		t.Fatalf("limit unexpected %+v", series.Limit)
	}
}

func TestAPIRecommendationsVerdicts(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	st := fixtureStore(t, now)
	insertRec(t, st, now)
	mux := liveMux(t, st, now)

	rr := authedGet(mux, "/api/v1/status", "")
	var stLive server.StatusOut
	_ = json.Unmarshal(rr.Body.Bytes(), &stLive)
	if rr.Code != http.StatusOK || !stLive.KubeLive {
		t.Fatalf("kube_live status %d %+v", rr.Code, stLive)
	}

	rr = authedGet(mux, "/api/v1/workloads?window=24h", "")
	var wl server.WorkloadsOut
	_ = json.Unmarshal(rr.Body.Bytes(), &wl)
	if rr.Code != http.StatusOK || wl.Count != 1 {
		t.Fatalf("workloads %d %+v", rr.Code, wl)
	}

	rr = authedGet(mux, "/api/v1/recommendations", "")
	var recs server.RecsOut
	if err := json.Unmarshal(rr.Body.Bytes(), &recs); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || recs.Count != 1 || len(recs.Rows) != 1 {
		t.Fatalf("recs %d %+v", rr.Code, recs)
	}
	row := recs.Rows[0]
	if row.CPUVerdict != "oversized" || row.MemVerdict != "oversized" {
		t.Fatalf("verdicts cpu=%q mem=%q", row.CPUVerdict, row.MemVerdict)
	}
	if row.CPUDeltaM != -100 || row.MemDeltaB != -1000 {
		t.Fatalf("deltas cpu=%v mem=%v", row.CPUDeltaM, row.MemDeltaB)
	}
}

func TestAPIWorkloadsNamespaceFilter(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	mux := liveMux(t, fixtureStore(t, now), now)

	rr := authedGet(mux, "/api/v1/workloads?window=24h&namespace=app&workload=api", "")
	var wl server.WorkloadsOut
	_ = json.Unmarshal(rr.Body.Bytes(), &wl)
	if rr.Code != http.StatusOK || wl.Count != 1 {
		t.Fatalf("workloads filtered %d %+v", rr.Code, wl)
	}
	rr = authedGet(mux, "/api/v1/workloads?window=24h&namespace=other", "")
	_ = json.Unmarshal(rr.Body.Bytes(), &wl)
	if rr.Code != http.StatusOK || wl.Count != 0 {
		t.Fatalf("workloads miss %d %+v", rr.Code, wl)
	}
}

func TestAPINodesOffline(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	st := fixtureStore(t, now)
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{Store: st, Version: "v", Now: func() time.Time { return now }}, "")

	rr := authedGet(mux, "/api/v1/nodes", "")
	var empty server.NodesOut
	_ = json.Unmarshal(rr.Body.Bytes(), &empty)
	if rr.Code != http.StatusOK || empty.Count != 0 {
		t.Fatalf("empty nodes %d %+v", rr.Code, empty)
	}
	if err := st.InsertNodeSnapshots(context.Background(), []store.NodeSnapshot{{
		RecordedAt: now, ClientID: "c", ClusterName: "x", Node: "worker",
		CPUAllocMcores: 2000, CPUReqMcores: 500, MemAllocBytes: 4 << 30, Pods: 3,
	}}); err != nil {
		t.Fatal(err)
	}
	rr = authedGet(mux, "/api/v1/nodes", "")
	var out server.NodesOut
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if rr.Code != http.StatusOK || out.Count != 1 || out.Items[0].Node != "worker" {
		t.Fatalf("nodes %d %+v", rr.Code, out)
	}
	if out.Items[0].CPUReqMcores != 500 || out.Items[0].Pods != 3 {
		t.Fatalf("row %+v", out.Items[0])
	}
}

func TestAPILiveAvailableAndMissing(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	st := fixtureStore(t, now)
	mux := liveMux(t, st, now)
	rr := authedGet(mux, "/api/v1/live", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("live %d", rr.Code)
	}
	mux2 := http.NewServeMux()
	server.Mount(mux2, &server.Deps{Store: st, Version: "v"}, "")
	rr = authedGet(mux2, "/api/v1/live", "")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("live nil want 503 got %d", rr.Code)
	}
}

func TestAPIExportPatch(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	st := fixtureStore(t, now)
	if err := st.InsertRecommendations(context.Background(), []store.Recommendation{{
		RecordedAt: now, ClientID: "c", ClusterName: "x",
		Grain: "workload", GrainKey: "app/Deployment/web",
		Namespace: "app", OwnerKind: "Deployment", OwnerName: "web", Container: "main",
		Samples: 5, CPUSugMcores: 250, MemSugBytes: 128 * 1024 * 1024,
		CPUReqMcores: 500, MemReqBytes: 256 * 1024 * 1024, TargetUtilization: 0.7,
	}}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{Store: st, Version: "v"}, "")

	assertCode(t, authedGet(mux, "/api/v1/export/patch", ""), http.StatusBadRequest)
	assertCode(t, authedGet(mux, "/api/v1/export/patch?grain_key=missing", ""), http.StatusNotFound)

	rr := authedGet(mux, "/api/v1/export/patch?grain_key=app/Deployment/web&format=strategic", "")
	assertCode(t, rr, http.StatusOK)
	var out server.PatchExportOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Format != "strategic" || out.GrainKey != "app/Deployment/web" || out.Content == "" {
		t.Fatalf("%+v", out)
	}
	if !containsAll(out.Content, "kind: Deployment", `cpu: "250m"`, "do not kubectl apply") {
		t.Fatalf("content:\n%s", out.Content)
	}

	rr = authedGet(mux, "/api/v1/export/patch?grain_key=app/Deployment/web&format=kustomize", "")
	assertCode(t, rr, http.StatusOK)
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.Format != "kustomize" || !containsAll(out.Content, "kustomization.yaml") {
		t.Fatalf("%+v", out)
	}
	assertCode(t, authedGet(mux, "/api/v1/export/patch?grain_key=app/Deployment/web&format=nope", ""), http.StatusBadRequest)

	muxNil := http.NewServeMux()
	server.Mount(muxNil, &server.Deps{Version: "v"}, "")
	assertCode(t, authedGet(muxNil, "/api/v1/export/patch?grain_key=x", ""), http.StatusServiceUnavailable)
}

func insertRec(t *testing.T, st *store.Store, now time.Time) {
	t.Helper()
	if err := st.InsertRecommendations(context.Background(), []store.Recommendation{{
		RecordedAt: now, ClientID: "c", ClusterName: "x",
		Grain: "workload", GrainKey: "app/StatefulSet/api",
		Namespace: "app", OwnerKind: "StatefulSet", OwnerName: "api",
		Samples: 10, CPUPMcores: 40, MemPBytes: 400,
		CPUSugMcores: 100, MemSugBytes: 1000,
		CPUReqMcores: 200, MemReqBytes: 2000, TargetUtilization: 0.7,
	}}); err != nil {
		t.Fatal(err)
	}
}

func liveMux(t *testing.T, st *store.Store, now time.Time) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{
		Store: st, Version: "v",
		Now: func() time.Time { return now },
		Live: func(context.Context) (server.LiveOut, error) {
			return server.LiveOut{CapturedAt: now.Format(time.RFC3339), Count: 1, Rows: []server.LiveRow{{
				Namespace: "app", Pod: "api-0", Container: "main", HasUtil: true,
			}}}, nil
		},
	}, "")
	return mux
}

func assertCode(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("want %d got %d %s", want, rr.Code, rr.Body.String())
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func TestResolveAPITokenAndBadMetric(t *testing.T) {
	t.Setenv("KPM_SERVER_TOKEN", "tok")
	cfg := &config.Config{}
	cfg.Server.TokenEnv = "KPM_SERVER_TOKEN"
	tok, err := server.ResolveAPIToken(cfg)
	if err != nil || tok != "tok" {
		t.Fatalf("token %q err=%v", tok, err)
	}
	cfg2 := &config.Config{}
	cfg2.MCP.TokenEnv = "KPM_SERVER_TOKEN"
	tok, err = server.ResolveAPIToken(cfg2)
	if err != nil || tok != "tok" {
		t.Fatalf("shared mcp token %q err=%v", tok, err)
	}

	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{Store: fixtureStore(t, now), Version: "v", Now: func() time.Time { return now }}, "")
	rr := authedGet(mux, "/api/v1/series?metric=nope&window=1h", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad metric %d", rr.Code)
	}
}

func TestAPIErrorPaths(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{Version: "v", Now: func() time.Time { return now }}, "")
	for _, path := range []string{"/api/v1/workloads", "/api/v1/series?window=1h", "/api/v1/recommendations"} {
		rr := authedGet(mux, path, "")
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s want 503 got %d", path, rr.Code)
		}
	}
	st := fixtureStore(t, now)
	mux2 := http.NewServeMux()
	server.Mount(mux2, &server.Deps{Store: st, Version: "v", Now: func() time.Time { return now }, Live: func(context.Context) (server.LiveOut, error) {
		return server.LiveOut{}, context.DeadlineExceeded
	}}, "")
	rr := authedGet(mux2, "/api/v1/live", "")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("live err %d", rr.Code)
	}
	rr = authedGet(mux2, "/api/v1/series?window=nope", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad window %d", rr.Code)
	}
	rr = authedGet(mux2, "/api/v1/series?since=bad", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad since %d", rr.Code)
	}
	rr = authedGet(mux2, "/api/v1/series?window=1h&max_points=0", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad max_points %d", rr.Code)
	}
	rr = authedGet(mux2, "/api/v1/series?metric=mem_util_bytes&window=30m", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("mem bytes / go duration window %d %s", rr.Code, rr.Body.String())
	}
	rr = authedGet(mux2, "/api/v1/series?window=720h", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("oversize window want 400, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestSeriesDownsampleAndRatios(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var recs []store.Record
	for i := 0; i < 20; i++ {
		recs = append(recs, store.Record{
			SampledAt: now.Add(-time.Duration(20-i) * time.Minute),
			Namespace: "app", Pod: "p", Container: "c", OwnerName: "w",
			CPUUtilMcores: float64(i), MemUtilBytes: int64(i * 10),
			CPURequestMcores: 10, MemRequestBytes: 100,
		})
	}
	if err := st.InsertRecords(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{Store: st, Version: "v", Now: func() time.Time { return now }}, "")
	rr := authedGet(mux, "/api/v1/series?metric=cpu_util_ratio&window=1h&max_points=5", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var series server.SeriesOut
	_ = json.Unmarshal(rr.Body.Bytes(), &series)
	if !series.Downsampled || series.Count != 5 {
		t.Fatalf("%+v", series)
	}
	rr = authedGet(mux, "/api/v1/series?metric=mem_util_ratio&window=1h", "")
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code)
	}
}

func TestAPISeriesBatchRejectsTwentyOneWorkloads(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{
		Store: fixtureStore(t, now), Version: "v",
		Now: func() time.Time { return now },
	}, "secret")
	var parts []string
	for i := 0; i < 21; i++ {
		parts = append(parts, fmt.Sprintf("app/w%d", i))
	}
	rr := authedGet(mux, "/api/v1/series/batch?metric=cpu_util_mcores&workloads="+strings.Join(parts, ","), "secret")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "at most 20") {
		t.Fatalf("body %s", rr.Body.String())
	}
}
