// Package live builds live Pod+Metrics snapshots for capacity, TUI, and server.
package live

// ContainerRow is one container after joining a Pod with Metrics.
type ContainerRow struct {
	Namespace string
	Pod       string
	Container string
	OwnerKind string
	OwnerName string
	Node      string

	CPUUtilMcores    float64
	CPURequestMcores float64
	CPULimitMcores   float64
	MemUtilBytes     int64
	MemRequestBytes  int64
	MemLimitBytes    int64

	// HasUtil is false when Metrics API had no sample for this container.
	HasUtil bool

	// PodCPURequestMcores / PodMemRequestBytes are the pod's *effective* request
	// — max(sum of regular containers, max of init containers) — repeated on
	// every row of that pod. Kubernetes reserves that number, not the container
	// sum, so node totals must use it. The per-container fields above stay per
	// container, because rightsizing advice is per container and an init
	// container consumes nothing at runtime.
	PodCPURequestMcores float64
	PodMemRequestBytes  int64
}

// PodRow is pod-aggregated (sum util / sum request / sum limit) for capacity/TUI.
type PodRow struct {
	Namespace string
	Pod       string
	OwnerKind string
	OwnerName string
	Node      string

	CPUUtilMcores    float64
	CPURequestMcores float64
	CPULimitMcores   float64
	MemUtilBytes     int64
	MemRequestBytes  int64
	MemLimitBytes    int64

	// HasUtil is true if any container had util.
	HasUtil bool
}

// Ratio returns util/denom and ok=false when denom is 0 or util is missing.
func Ratio(util, denom float64, hasUtil bool) (float64, bool) {
	if !hasUtil || denom <= 0 {
		return 0, false
	}
	return util / denom, true
}

// CPURatioRequest is util/request for a container row.
func (r ContainerRow) CPURatioRequest() (float64, bool) {
	return Ratio(r.CPUUtilMcores, r.CPURequestMcores, r.HasUtil)
}

// CPURatioLimit is util/limit for a container row.
func (r ContainerRow) CPURatioLimit() (float64, bool) {
	return Ratio(r.CPUUtilMcores, r.CPULimitMcores, r.HasUtil)
}

// MemRatioRequest is util/request for a container row.
func (r ContainerRow) MemRatioRequest() (float64, bool) {
	return Ratio(float64(r.MemUtilBytes), float64(r.MemRequestBytes), r.HasUtil)
}

// MemRatioLimit is util/limit for a container row.
func (r ContainerRow) MemRatioLimit() (float64, bool) {
	return Ratio(float64(r.MemUtilBytes), float64(r.MemLimitBytes), r.HasUtil)
}

// CPURatioRequest is util/request for a pod row.
func (r PodRow) CPURatioRequest() (float64, bool) {
	return Ratio(r.CPUUtilMcores, r.CPURequestMcores, r.HasUtil)
}

// CPURatioLimit is util/limit for a pod row.
func (r PodRow) CPURatioLimit() (float64, bool) {
	return Ratio(r.CPUUtilMcores, r.CPULimitMcores, r.HasUtil)
}

// MemRatioRequest is util/request for a pod row.
func (r PodRow) MemRatioRequest() (float64, bool) {
	return Ratio(float64(r.MemUtilBytes), float64(r.MemRequestBytes), r.HasUtil)
}

// MemRatioLimit is util/limit for a pod row.
func (r PodRow) MemRatioLimit() (float64, bool) {
	return Ratio(float64(r.MemUtilBytes), float64(r.MemLimitBytes), r.HasUtil)
}
