package live

import (
	"sort"
	"strings"
)

// GeneralPool labels the nodes that no blocking taint keeps anyone off.
const GeneralPool = "(general)"

// PoolKey names the pool a node belongs to: the set of blocking taints, which is
// exactly what decides who may use its capacity. Nodes with no blocking taint all
// share the general pool.
//
// This is a grouping for reading, not a filter. A dedicated pool's capacity is
// real and appears in the totals; the pool name says who can consume it.
func PoolKey(n NodeCapacity) string {
	if len(n.Taints) == 0 {
		return GeneralPool
	}
	taints := append([]string(nil), n.Taints...)
	sort.Strings(taints)
	return strings.Join(taints, ",")
}

// PoolSummary is one pool's share of the cluster.
type PoolSummary struct {
	Pool           string
	Nodes          int
	Cordoned       int
	CPUAllocMcores float64
	CPUCapMcores   float64
	CPUReqMcores   float64
	CPUUtilMcores  float64
	MemAllocBytes  int64
	MemCapBytes    int64
	MemReqBytes    int64
	MemUtilBytes   int64
	Pods           int
}

// GroupNodesByPool aggregates node capacity per pool, sorted by pool name so the
// output is stable across runs.
func GroupNodesByPool(nodes []NodeCapacity) []PoolSummary {
	byPool := make(map[string]*PoolSummary, len(nodes))
	for _, n := range nodes {
		key := PoolKey(n)
		p := byPool[key]
		if p == nil {
			p = &PoolSummary{Pool: key}
			byPool[key] = p
		}
		p.Nodes++
		if n.Unschedulable {
			p.Cordoned++
		}
		p.CPUAllocMcores += n.CPUAllocMcores
		p.CPUCapMcores += n.CPUCapMcores
		p.CPUReqMcores += n.CPUReqMcores
		p.CPUUtilMcores += n.CPUUtilMcores
		p.MemAllocBytes += n.MemAllocBytes
		p.MemCapBytes += n.MemCapBytes
		p.MemReqBytes += n.MemReqBytes
		p.MemUtilBytes += n.MemUtilBytes
		p.Pods += n.Pods
	}

	out := make([]PoolSummary, 0, len(byPool))
	for _, p := range byPool {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pool < out[j].Pool })
	return out
}
