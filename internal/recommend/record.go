package recommend

import (
	"time"

	"github.com/hrodrig/kpm/internal/store"
)

// SnapshotsFromResult builds recommendation rows to persist from a Run result.
func SnapshotsFromResult(res Result, clientID, clusterName string, at time.Time) []store.Recommendation {
	out := make([]store.Recommendation, 0, len(res.Advice))
	recorded := at.UTC()
	windowDays := daysBetween(res.WindowStart, res.WindowEnd)
	for _, a := range res.Advice {
		out = append(out, store.Recommendation{
			RecordedAt:        recorded,
			ClientID:          clientID,
			ClusterName:       clusterName,
			Grain:             string(a.Grain),
			GrainKey:          a.Key,
			Namespace:         a.Namespace,
			Pod:               a.Pod,
			Container:         a.Container,
			OwnerKind:         a.OwnerKind,
			OwnerName:         a.OwnerName,
			Samples:           a.Samples,
			Percentile:        res.Percentile,
			TargetUtilization: res.TargetUtil,
			// Persist the resolved per-resource targets so --score labels post-change
			// utilization against the target the suggestion was actually built with.
			CPUTargetUtilization:    res.CPUTargetUtil,
			MemoryTargetUtilization: res.MemTargetUtil,
			WindowDays:              windowDays,
			CPUPMcores:              a.CPUPercentile,
			MemPBytes:               int64(a.MemPercentile),
			CPUSugMcores:            a.CPUSuggested,
			MemSugBytes:             a.MemSuggested,
			CPUReqMcores:            a.CPURequest,
			MemReqBytes:             a.MemRequest,
		})
	}
	return out
}

func daysBetween(start, end time.Time) int {
	d := int(end.Sub(start).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// Delta is live allocation vs last suggested request.
type Delta struct {
	HasSnapshot  bool
	CPUReqDelta  float64 // liveCPUReq - suggested
	MemReqDelta  int64   // liveMemReq - suggested
	CPUSugMcores float64
	MemSugBytes  int64
	RecordedAt   time.Time
}

// DeltaFromLive compares live requests to a persisted snapshot.
func DeltaFromLive(liveCPUReq float64, liveMemReq int64, snap *store.Recommendation) Delta {
	if snap == nil {
		return Delta{}
	}
	return Delta{
		HasSnapshot:  true,
		CPUReqDelta:  liveCPUReq - snap.CPUSugMcores,
		MemReqDelta:  liveMemReq - snap.MemSugBytes,
		CPUSugMcores: snap.CPUSugMcores,
		MemSugBytes:  snap.MemSugBytes,
		RecordedAt:   snap.RecordedAt,
	}
}
