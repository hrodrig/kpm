package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/store"
)

func toolText(res *mcpsdk.CallToolResult) string {
	var msg string
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			msg += tc.Text
		}
	}
	return msg
}

func TestServerToolsInMemory(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if err := st.InsertRecords(context.Background(), []store.Record{{
		SampledAt: now, Namespace: "app", Pod: "p", Container: "c",
		CPUUtilMcores: 50, CPURequestMcores: 100,
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.MCP.Enabled = true
	cfg.Sample.IntervalSeconds = 60
	d := &mcp.Deps{Store: st, Config: cfg, Version: "v-test"}
	session := connectMCP(t, d)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "get_status"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolText(res))
	}
	b, _ := json.Marshal(res.StructuredContent)
	if !json.Valid(b) || len(b) < 10 {
		t.Fatalf("structured: %s", b)
	}

	res2, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "list_samples",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.IsError {
		t.Fatalf("list_samples: %s", toolText(res2))
	}
}

func connectMCP(t *testing.T, d *mcp.Deps) *mcpsdk.ClientSession {
	t.Helper()
	srv := mcp.NewServer(d)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0"}, nil)
	t1, t2 := mcpsdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestRunStdioRequiresStore(t *testing.T) {
	if err := mcp.RunStdio(context.Background(), &mcp.Deps{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterAndEdges(t *testing.T) {
	d := &mcp.Deps{}
	if _, err := d.ListSamples(context.Background(), mcp.ListSamplesIn{}); err == nil {
		t.Fatal("nil store")
	}
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	d.Store = st
	if _, err := d.ListSamples(context.Background(), mcp.ListSamplesIn{Since: "bad"}); err == nil {
		t.Fatal("bad since")
	}
	if _, err := d.ListSamples(context.Background(), mcp.ListSamplesIn{Until: "bad"}); err == nil {
		t.Fatal("bad until")
	}
	_ = mcp.MustJSON(map[string]int{"a": 1})
	stOut, err := d.GetStatus(context.Background())
	if err != nil || stOut.StorePath == "" {
		t.Fatalf("status: %+v err=%v", stOut, err)
	}
}
