package live

import "sort"

// AggregatePods sums container rows into one PodRow per namespace/pod
// (sum util / sum request / sum limit).
func AggregatePods(containers []ContainerRow) []PodRow {
	type key struct{ ns, pod string }
	idx := map[key]int{}
	var out []PodRow
	for _, c := range containers {
		k := key{c.Namespace, c.Pod}
		i, ok := idx[k]
		if !ok {
			out = append(out, PodRow{
				Namespace: c.Namespace,
				Pod:       c.Pod,
				OwnerKind: c.OwnerKind,
				OwnerName: c.OwnerName,
				Node:      c.Node,
			})
			i = len(out) - 1
			idx[k] = i
		}
		r := &out[i]
		r.CPURequestMcores += c.CPURequestMcores
		r.CPULimitMcores += c.CPULimitMcores
		r.MemRequestBytes += c.MemRequestBytes
		r.MemLimitBytes += c.MemLimitBytes
		if c.HasUtil {
			r.HasUtil = true
			r.CPUUtilMcores += c.CPUUtilMcores
			r.MemUtilBytes += c.MemUtilBytes
		}
		if r.Node == "" {
			r.Node = c.Node
		}
		if r.OwnerKind == "" {
			r.OwnerKind = c.OwnerKind
			r.OwnerName = c.OwnerName
		}
	}
	return out
}

// SortKey is a capacity/TUI sort key.
type SortKey string

const (
	SortName     SortKey = "name"
	SortCPURatio SortKey = "cpu.ratio"
	SortCPUUtil  SortKey = "cpu.util"
	SortMemRatio SortKey = "mem.ratio"
	SortMemUtil  SortKey = "mem.util"
)

// SortPods sorts pod rows in place. Unknown keys fall back to cpu.ratio.
func SortPods(rows []PodRow, key SortKey) {
	less := func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch key {
		case SortName:
			if a.Namespace != b.Namespace {
				return a.Namespace < b.Namespace
			}
			return a.Pod < b.Pod
		case SortCPUUtil:
			return a.CPUUtilMcores > b.CPUUtilMcores
		case SortMemUtil:
			return a.MemUtilBytes > b.MemUtilBytes
		case SortMemRatio:
			ra, oka := a.MemRatioRequest()
			rb, okb := b.MemRatioRequest()
			return compareRatioDesc(ra, oka, rb, okb, a, b)
		default: // SortCPURatio
			ra, oka := a.CPURatioRequest()
			rb, okb := b.CPURatioRequest()
			return compareRatioDesc(ra, oka, rb, okb, a, b)
		}
	}
	sort.SliceStable(rows, less)
}

func compareRatioDesc(ra float64, oka bool, rb float64, okb bool, a, b PodRow) bool {
	if oka != okb {
		return oka // rows with ratio first
	}
	if oka && ra != rb {
		return ra > rb
	}
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	return a.Pod < b.Pod
}
