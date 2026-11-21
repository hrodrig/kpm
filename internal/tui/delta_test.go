package tui

import (
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/store"
)

func TestDeltaForPodAndFormat(t *testing.T) {
	idx := IndexRecommendations([]store.Recommendation{{
		Grain: "pod", GrainKey: "app/web-0/c",
		Namespace: "app", Pod: "web-0", Container: "c",
		CPUSugMcores: 80, MemSugBytes: 32 << 20,
		RecordedAt: time.Now().UTC(),
	}})
	pod := live.PodRow{Namespace: "app", Pod: "web-0", CPURequestMcores: 100, MemRequestBytes: 64 << 20}
	containers := []live.ContainerRow{{
		Namespace: "app", Pod: "web-0", Container: "c",
		CPURequestMcores: 100, MemRequestBytes: 64 << 20,
	}}
	d := DeltaForPod(pod, containers, idx)
	if !d.HasSnapshot || d.CPUReqDelta != 20 || d.MemReqDelta != 32<<20 {
		t.Fatalf("delta: %+v", d)
	}
	if FormatCPUDelta(d) != "+20m" {
		t.Fatalf("cpu fmt: %s", FormatCPUDelta(d))
	}
	if FormatMemDelta(d) != "+32Mi" {
		t.Fatalf("mem fmt: %s", FormatMemDelta(d))
	}
	if FormatCPUDelta(PodDelta{}) != "n/a" {
		t.Fatal("na")
	}
}

func TestDeltaWorkloadFallback(t *testing.T) {
	idx := IndexRecommendations([]store.Recommendation{{
		Grain: "workload", GrainKey: "Deployment/web/c",
		OwnerKind: "Deployment", OwnerName: "web", Container: "c",
		CPUSugMcores: 50, MemSugBytes: 10 << 20,
	}})
	pod := live.PodRow{Namespace: "app", Pod: "web-0"}
	containers := []live.ContainerRow{{
		Namespace: "app", Pod: "web-0", Container: "c",
		OwnerKind: "Deployment", OwnerName: "web",
		CPURequestMcores: 100, MemRequestBytes: 20 << 20,
	}}
	d := DeltaForPod(pod, containers, idx)
	if !d.HasSnapshot || d.CPUReqDelta != 50 {
		t.Fatalf("workload delta: %+v", d)
	}
}

func TestDeltaWorkloadNamespaceIsolation(t *testing.T) {
	idx := IndexRecommendations([]store.Recommendation{
		{
			Grain: "workload", GrainKey: "prod/Deployment/api/app",
			Namespace: "prod", OwnerKind: "Deployment", OwnerName: "api", Container: "app",
			CPUSugMcores: 500, MemSugBytes: 512 << 20,
		},
		{
			Grain: "workload", GrainKey: "staging/Deployment/api/app",
			Namespace: "staging", OwnerKind: "Deployment", OwnerName: "api", Container: "app",
			CPUSugMcores: 50, MemSugBytes: 64 << 20,
		},
		{
			Grain: "workload", GrainKey: "Deployment/legacy/app",
			Namespace: "staging", OwnerKind: "Deployment", OwnerName: "legacy", Container: "app",
			CPUSugMcores: 25, MemSugBytes: 32 << 20,
		},
	})
	pod := live.PodRow{Namespace: "staging", Pod: "api-0"}
	containers := []live.ContainerRow{{
		Namespace: "staging", Pod: "api-0", Container: "app",
		OwnerKind: "Deployment", OwnerName: "api",
		CPURequestMcores: 100, MemRequestBytes: 128 << 20,
	}}
	d := DeltaForPod(pod, containers, idx)
	if !d.HasSnapshot || d.CPUReqDelta != 50 {
		t.Fatalf("staging should match staging workload rec (delta 50), got %+v", d)
	}

	// Unrelated namespace must not borrow prod's recommendation.
	otherPod := live.PodRow{Namespace: "other", Pod: "api-0"}
	otherC := []live.ContainerRow{{
		Namespace: "other", Pod: "api-0", Container: "app",
		OwnerKind: "Deployment", OwnerName: "api",
		CPURequestMcores: 100, MemRequestBytes: 128 << 20,
	}}
	if d2 := DeltaForPod(otherPod, otherC, idx); d2.HasSnapshot {
		t.Fatalf("other namespace must not match prod/staging rec: %+v", d2)
	}
}
