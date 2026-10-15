package sample

import (
	"context"

	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/store"
)

// ResolveOwners walks ReplicaSet owners up to Deployment for stable grains.
// Failures leave the original RS identity (best-effort).
func ResolveOwners(ctx context.Context, g live.WorkloadGetter, records []store.Record) {
	if g == nil || len(records) == 0 {
		return
	}
	type resolved struct{ kind, name string }
	cache := make(map[string]resolved)
	for i := range records {
		if records[i].OwnerKind != "ReplicaSet" || records[i].OwnerName == "" {
			continue
		}
		key := records[i].Namespace + "/" + records[i].OwnerName
		if hit, ok := cache[key]; ok {
			records[i].OwnerKind, records[i].OwnerName = hit.kind, hit.name
			continue
		}
		ot, err := live.ResolveOwnerTemplate(ctx, g, records[i].Namespace, records[i].OwnerKind, records[i].OwnerName)
		if err != nil || ot.Unsupported || ot.Kind == "" || ot.Name == "" {
			cache[key] = resolved{kind: records[i].OwnerKind, name: records[i].OwnerName}
			continue
		}
		cache[key] = resolved{kind: ot.Kind, name: ot.Name}
		records[i].OwnerKind, records[i].OwnerName = ot.Kind, ot.Name
	}
}
