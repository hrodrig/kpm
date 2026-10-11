package tui

import (
	"strings"
	"testing"

	"github.com/hrodrig/kpm/internal/live"
)

func TestFormatOwnerDetailCompare(t *testing.T) {
	tmpl := live.OwnerTemplate{
		Kind: "Deployment", Name: "web", Namespace: "app", Via: "ReplicaSet/web-x",
		Containers: []live.ContainerResources{{
			Name: "c", CPURequestMcores: 100, MemRequestBytes: 64 << 20,
			CPULimitMcores: 200, MemLimitBytes: 128 << 20,
		}},
	}
	liveRows := []live.ContainerResources{{
		Name: "c", CPURequestMcores: 150, MemRequestBytes: 64 << 20,
		CPULimitMcores: 200, MemLimitBytes: 128 << 20,
	}}
	s := FormatOwnerDetail("app", "web-0", tmpl, liveRows)
	if !strings.Contains(s, "Deployment/web") || !strings.Contains(s, "template ≠ live") {
		t.Fatalf("detail: %s", s)
	}
	if !strings.Contains(s, "[esc] close") {
		t.Fatal("esc hint")
	}
}

func TestFormatOwnerDetailUnsupported(t *testing.T) {
	s := FormatOwnerDetail("a", "p", live.OwnerTemplate{
		Unsupported: true, UnsupportedReason: "owner kind Job not Deploy/STS/DS",
	}, nil)
	if !strings.Contains(s, "n/a") {
		t.Fatalf("%s", s)
	}
}
