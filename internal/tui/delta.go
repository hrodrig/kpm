package tui

import (
	"fmt"

	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/recommend"
	"github.com/hrodrig/kpm/internal/store"
)

// RecIndex indexes latest recommendation snapshots for TUI joins.
type RecIndex struct {
	ByPodContainer map[string]store.Recommendation // ns/pod/container
	ByWorkload     map[string]store.Recommendation // kind/name/container
}

// IndexRecommendations builds lookup maps from latest snapshots.
func IndexRecommendations(recs []store.Recommendation) RecIndex {
	idx := RecIndex{
		ByPodContainer: make(map[string]store.Recommendation, len(recs)),
		ByWorkload:     make(map[string]store.Recommendation, len(recs)),
	}
	for _, r := range recs {
		switch r.Grain {
		case string(recommend.GrainPod):
			idx.ByPodContainer[r.GrainKey] = r
		case string(recommend.GrainWorkload):
			idx.ByWorkload[r.GrainKey] = r
		}
	}
	return idx
}

// PodDelta is aggregated live request − suggested for a pod row.
type PodDelta struct {
	HasSnapshot bool
	CPUReqDelta float64
	MemReqDelta int64
}

// DeltaForPod joins container rows of a pod to latest recommendations.
// Uses pod-grain key first, then workload-grain; sums matched containers.
func DeltaForPod(pod live.PodRow, containers []live.ContainerRow, idx RecIndex) PodDelta {
	var liveCPU, sugCPU float64
	var liveMem, sugMem int64
	matched := 0
	for _, c := range containers {
		if c.Namespace != pod.Namespace || c.Pod != pod.Pod {
			continue
		}
		snap, ok := lookupRec(c, idx)
		if !ok {
			continue
		}
		matched++
		liveCPU += c.CPURequestMcores
		liveMem += c.MemRequestBytes
		sugCPU += snap.CPUSugMcores
		sugMem += snap.MemSugBytes
	}
	if matched == 0 {
		return PodDelta{}
	}
	d := recommend.DeltaFromLive(liveCPU, liveMem, &store.Recommendation{
		CPUSugMcores: sugCPU,
		MemSugBytes:  sugMem,
	})
	return PodDelta{
		HasSnapshot: true,
		CPUReqDelta: d.CPUReqDelta,
		MemReqDelta: d.MemReqDelta,
	}
}

func lookupRec(c live.ContainerRow, idx RecIndex) (store.Recommendation, bool) {
	if r, ok := idx.ByPodContainer[c.Namespace+"/"+c.Pod+"/"+c.Container]; ok {
		return r, true
	}
	if c.OwnerKind != "" && c.OwnerName != "" {
		if r, ok := idx.ByWorkload[c.OwnerKind+"/"+c.OwnerName+"/"+c.Container]; ok {
			return r, true
		}
	}
	return store.Recommendation{}, false
}

// FormatCPUDelta formats millicores delta or n/a.
func FormatCPUDelta(d PodDelta) string {
	if !d.HasSnapshot {
		return "n/a"
	}
	return fmt.Sprintf("%+.0fm", d.CPUReqDelta)
}

// FormatMemDelta formats bytes delta as MiB or n/a.
func FormatMemDelta(d PodDelta) string {
	if !d.HasSnapshot {
		return "n/a"
	}
	mib := float64(d.MemReqDelta) / (1 << 20)
	return fmt.Sprintf("%+.0fMi", mib)
}
