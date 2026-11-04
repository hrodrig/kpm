package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// storeToolAnnotations declares all four MCP hints as explicit booleans.
// Tools read the local SQLite store only (no cluster writes, no open-world I/O).
func storeToolAnnotations() *mcpsdk.ToolAnnotations {
	f := false
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: &f,
		IdempotentHint:  true,
		OpenWorldHint:   &f,
	}
}

// NewServer builds an MCP server with the four store-only tools registered.
func NewServer(d *Deps) *mcpsdk.Server {
	ver := d.Version
	if ver == "" {
		ver = "dev"
	}
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "kpm", Version: ver}, nil)
	RegisterTools(s, d)
	return s
}

// RegisterTools attaches read-only tools to s.
func RegisterTools(s *mcpsdk.Server, d *Deps) {
	ann := storeToolAnnotations()

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_status",
		Description: "kpm status: version, store path, sample interval, notify/mcp flags (no secrets)",
		Annotations: ann,
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, StatusOut, error) {
		out, err := d.GetStatus(ctx)
		return nil, out, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_samples",
		Description: "List store sample rows (namespace/workload/time filters; hard row cap)",
		Annotations: ann,
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListSamplesIn) (*mcpsdk.CallToolResult, ListSamplesOut, error) {
		out, err := d.ListSamples(ctx, in)
		return nil, out, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "summarize_window",
		Description: "Aggregate util ratios over store samples in a window (no live Metrics API)",
		Annotations: ann,
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in SummarizeIn) (*mcpsdk.CallToolResult, SummarizeOut, error) {
		out, err := d.SummarizeWindow(ctx, in)
		return nil, out, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_recommendations",
		Description: "Latest rightsizing recommendation snapshots from the store",
		Annotations: ann,
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListRecsIn) (*mcpsdk.CallToolResult, ListRecsOut, error) {
		out, err := d.ListRecommendations(ctx, in)
		return nil, out, err
	})
}

// RunStdio serves MCP over stdin/stdout until the client disconnects.
func RunStdio(ctx context.Context, d *Deps) error {
	if d == nil || d.Store == nil {
		return fmt.Errorf("mcp: store required")
	}
	return NewServer(d).Run(ctx, &mcpsdk.StdioTransport{})
}
