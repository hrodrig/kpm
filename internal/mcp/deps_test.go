package mcp_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/store"
)

func openFixture(t *testing.T) (*store.Store, time.Time) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	recs := []store.Record{
		{
			SampledAt: now.Add(-2 * time.Hour), ClientID: "c", ClusterName: "k",
			Namespace: "app", Pod: "api-0", Container: "api", OwnerName: "api",
			CPUUtilMcores: 80, CPURequestMcores: 100,
			MemUtilBytes: 400, MemRequestBytes: 1000,
		},
		{
			SampledAt: now.Add(-1 * time.Hour), ClientID: "c", ClusterName: "k",
			Namespace: "app", Pod: "api-0", Container: "api", OwnerName: "api",
			CPUUtilMcores: 90, CPURequestMcores: 100,
			MemUtilBytes: 500, MemRequestBytes: 1000,
		},
		{
			SampledAt: now, ClientID: "c", ClusterName: "k",
			Namespace: "other", Pod: "x", Container: "x", OwnerName: "x",
			CPUUtilMcores: 10, CPURequestMcores: 100,
		},
	}
	if err := st.InsertRecords(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertRecommendations(context.Background(), []store.Recommendation{{
		RecordedAt: now, ClientID: "c", ClusterName: "k",
		Grain: "pod", GrainKey: "app/api", Namespace: "app", OwnerName: "api",
		Samples: 2, Percentile: 95, TargetUtilization: 0.7, WindowDays: 7,
		CPUSugMcores: 120, MemSugBytes: 1200, CPUReqMcores: 100, MemReqBytes: 1000,
	}}); err != nil {
		t.Fatal(err)
	}
	return st, now
}

func TestGetStatusAndList(t *testing.T) {
	st, now := openFixture(t)
	cfg := &config.Config{}
	cfg.Sample.IntervalSeconds = 300
	cfg.Notifications.Enabled = true
	cfg.MCP.Enabled = true
	cfg.Cluster.Name = "example"
	d := &mcp.Deps{Store: st, Config: cfg, Version: "v0.5.0-test", Now: func() time.Time { return now }}

	stOut, err := d.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stOut.Version != "v0.5.0-test" || !stOut.MCPEnabled || stOut.SampleIntervalSec != 300 {
		t.Fatalf("status: %+v", stOut)
	}
	list, err := d.ListSamples(context.Background(), mcp.ListSamplesIn{Namespace: "app", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Count != 2 || list.Truncated {
		t.Fatalf("list: %+v", list)
	}
}

func TestSummarizeAndRecs(t *testing.T) {
	st, _ := openFixture(t)
	d := &mcp.Deps{Store: st}
	sum, err := d.SummarizeWindow(context.Background(), mcp.SummarizeIn{Namespace: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if sum.SampleCount != 2 || sum.WithCPUReq != 2 || sum.CPUUtilAvg < 0.8 {
		t.Fatalf("sum: %+v", sum)
	}
	recsOut, err := d.ListRecommendations(context.Background(), mcp.ListRecsIn{Grain: "pod"})
	if err != nil {
		t.Fatal(err)
	}
	if recsOut.Count != 1 || recsOut.Rows[0].OwnerName != "api" {
		t.Fatalf("recs: %+v", recsOut)
	}
}

func TestListSamplesCap(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var recs []store.Record
	for i := 0; i < 10; i++ {
		recs = append(recs, store.Record{
			SampledAt: base.Add(time.Duration(i) * time.Minute),
			Namespace: "ns", Pod: "p", Container: "c",
			CPUUtilMcores: 1, CPURequestMcores: 10,
		})
	}
	if err := st.InsertRecords(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	d := &mcp.Deps{Store: st}
	out, err := d.ListSamples(context.Background(), mcp.ListSamplesIn{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated || out.Count != 3 {
		t.Fatalf("cap: %+v", out)
	}
}

func TestNewServerRegisters(t *testing.T) {
	d := &mcp.Deps{Version: "t"}
	s := mcp.NewServer(d)
	if s == nil {
		t.Fatal("nil server")
	}
}
