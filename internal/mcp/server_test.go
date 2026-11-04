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

func openServerFixture(t *testing.T) *mcpsdk.ClientSession {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if err := st.InsertRecords(context.Background(), []store.Record{{
		SampledAt: now, Namespace: "app", Pod: "p", Container: "c",
		CPUUtilMcores: 50, CPURequestMcores: 100,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertRecommendations(context.Background(), []store.Recommendation{{
		RecordedAt: now, Grain: "pod", GrainKey: "app/p", Namespace: "app",
		Samples: 1, Percentile: 95, TargetUtilization: 0.7, WindowDays: 7,
		CPUSugMcores: 60, MemSugBytes: 1, CPUReqMcores: 100, MemReqBytes: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.MCP.Enabled = true
	cfg.Sample.IntervalSeconds = 60
	d := &mcp.Deps{Store: st, Config: cfg, Version: "v-test"}
	session := connectMCP(t, d)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func assertStoreAnnotations(t *testing.T, tool *mcpsdk.Tool) {
	t.Helper()
	ann := tool.Annotations
	if ann == nil {
		t.Fatalf("%s: missing annotations", tool.Name)
	}
	if !ann.ReadOnlyHint || !ann.IdempotentHint {
		t.Fatalf("%s: readOnly/idempotent: %+v", tool.Name, ann)
	}
	if ann.DestructiveHint == nil || *ann.DestructiveHint {
		t.Fatalf("%s: destructiveHint want false: %+v", tool.Name, ann)
	}
	if ann.OpenWorldHint == nil || *ann.OpenWorldHint {
		t.Fatalf("%s: openWorldHint want false: %+v", tool.Name, ann)
	}
}

func TestServerToolAnnotations(t *testing.T) {
	session := openServerFixture(t)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"get_status": true, "list_samples": true,
		"summarize_window": true, "list_recommendations": true,
	}
	if len(listed.Tools) != len(want) {
		t.Fatalf("tools count: got %d want %d", len(listed.Tools), len(want))
	}
	for _, tool := range listed.Tools {
		if !want[tool.Name] {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
		assertStoreAnnotations(t, tool)
	}
}

func mustCallTool(t *testing.T, session *mcpsdk.ClientSession, name string, args map[string]any) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: name, Arguments: args,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("%s: %s", name, toolText(res))
	}
	if name == "get_status" {
		b, _ := json.Marshal(res.StructuredContent)
		if !json.Valid(b) || len(b) < 10 {
			t.Fatalf("structured: %s", b)
		}
	}
}

func TestServerCallAllTools(t *testing.T) {
	session := openServerFixture(t)
	mustCallTool(t, session, "get_status", nil)
	mustCallTool(t, session, "list_samples", map[string]any{})
	mustCallTool(t, session, "summarize_window", map[string]any{"namespace": "app"})
	mustCallTool(t, session, "list_recommendations", map[string]any{"grain": "pod"})
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
