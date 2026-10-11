// Package sample maps Kubernetes pod + metrics into store records.
package sample

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/hrodrig/kpm/internal/store"
)

// Result is the outcome of one capture pass.
type Result struct {
	Records     []store.Record
	SkippedPods int
	SampledAt   time.Time
	// StoreWrite is time spent in SQLite InsertRecords (zero if not measured).
	StoreWrite time.Duration
}

// BuildRecords joins pods and pod metrics into container-grain records.
// Pods without metrics are skipped (count in SkippedPods). API errors are
// the caller's responsibility (fail closed before calling this).
func BuildRecords(sampledAt time.Time, pods []corev1.Pod, metrics []metricsv1beta1.PodMetrics) Result {
	at := sampledAt.UTC()
	byKey := make(map[string]metricsv1beta1.PodMetrics, len(metrics))
	for _, m := range metrics {
		byKey[m.Namespace+"/"+m.Name] = m
	}

	var out []store.Record
	skipped := 0
	for i := range pods {
		p := &pods[i]
		key := p.Namespace + "/" + p.Name
		m, ok := byKey[key]
		if !ok {
			skipped++
			continue
		}
		ownerKind, ownerName := ownerRef(p)
		reqCPU, reqMem, limCPU, limMem := requestsLimits(p)
		utilByContainer := make(map[string]metricsv1beta1.ContainerMetrics, len(m.Containers))
		for _, c := range m.Containers {
			utilByContainer[c.Name] = c
		}
		for _, c := range p.Spec.Containers {
			um, hasUtil := utilByContainer[c.Name]
			rec := store.Record{
				SampledAt:        at,
				Namespace:        p.Namespace,
				Pod:              p.Name,
				Container:        c.Name,
				OwnerKind:        ownerKind,
				OwnerName:        ownerName,
				Node:             p.Spec.NodeName,
				CPURequestMcores: milliCPU(reqCPU[c.Name]),
				MemRequestBytes:  bytesQty(reqMem[c.Name]),
				CPULimitMcores:   milliCPU(limCPU[c.Name]),
				MemLimitBytes:    bytesQty(limMem[c.Name]),
			}
			if hasUtil {
				rec.CPUUtilMcores = milliCPU(um.Usage.Cpu())
				rec.MemUtilBytes = bytesQty(um.Usage.Memory())
			}
			out = append(out, rec)
		}
	}
	return Result{Records: out, SkippedPods: skipped, SampledAt: at}
}

func ownerRef(p *corev1.Pod) (kind, name string) {
	if len(p.OwnerReferences) == 0 {
		return "", ""
	}
	o := p.OwnerReferences[0]
	return o.Kind, o.Name
}

func requestsLimits(p *corev1.Pod) (reqCPU, reqMem, limCPU, limMem map[string]*resource.Quantity) {
	reqCPU = map[string]*resource.Quantity{}
	reqMem = map[string]*resource.Quantity{}
	limCPU = map[string]*resource.Quantity{}
	limMem = map[string]*resource.Quantity{}
	for _, c := range p.Spec.Containers {
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			cp := q
			reqCPU[c.Name] = &cp
		}
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			cp := q
			reqMem[c.Name] = &cp
		}
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			cp := q
			limCPU[c.Name] = &cp
		}
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			cp := q
			limMem[c.Name] = &cp
		}
	}
	return reqCPU, reqMem, limCPU, limMem
}

func milliCPU(q *resource.Quantity) float64 {
	if q == nil {
		return 0
	}
	return float64(q.MilliValue())
}

func bytesQty(q *resource.Quantity) int64 {
	if q == nil {
		return 0
	}
	return q.Value()
}
