package live

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NodeLister lists cluster Nodes (narrow interface for tests).
type NodeLister interface {
	List(ctx context.Context, opts metav1.ListOptions) (*corev1.NodeList, error)
}

type k8sNodeLister struct {
	cs kubernetes.Interface
}

func (l k8sNodeLister) List(ctx context.Context, opts metav1.ListOptions) (*corev1.NodeList, error) {
	return l.cs.CoreV1().Nodes().List(ctx, opts)
}

// NodeCapacity is one node's allocatable/capacity plus summed pod request/util.
type NodeCapacity struct {
	Name           string
	CPUAllocMcores float64
	MemAllocBytes  int64
	CPUCapMcores   float64
	MemCapBytes    int64
	CPUReqMcores   float64
	MemReqBytes    int64
	CPUUtilMcores  float64
	MemUtilBytes   int64
	Pods           int

	// Pool identity. Taints holds only the effects that keep a pod off the node
	// unless it tolerates them (NoSchedule, NoExecute); PreferNoSchedule is a
	// hint, not a filter. Neither field changes a total -- capacity that exists
	// is capacity that exists -- they say who may use it.
	Taints        []string
	Unschedulable bool
}

// CaptureNodeCapacity lists Nodes and joins live container rows by Spec.NodeName.
func CaptureNodeCapacity(ctx context.Context, nodes NodeLister, containers []ContainerRow) ([]NodeCapacity, error) {
	if nodes == nil {
		return nil, fmt.Errorf("live: node lister required")
	}
	nl, err := nodes.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	return AggregateNodes(nl.Items, containers), nil
}

// CaptureNodeCapacityClients uses a typed clientset for Nodes.
func CaptureNodeCapacityClients(ctx context.Context, cs kubernetes.Interface, containers []ContainerRow) ([]NodeCapacity, error) {
	return CaptureNodeCapacity(ctx, k8sNodeLister{cs: cs}, containers)
}

// BlockingTaints formats the taints that keep a pod off a node unless it tolerates
// them. PreferNoSchedule is a preference, not a filter: reporting it as blocking
// would hide capacity that is perfectly usable.
func BlockingTaints(taints []corev1.Taint) []string {
	var out []string
	for _, t := range taints {
		if t.Effect != corev1.TaintEffectNoSchedule && t.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		s := t.Key
		if t.Value != "" {
			s += "=" + t.Value
		}
		out = append(out, s+":"+string(t.Effect))
	}
	return out
}

// ContainerRowsFromPods builds the container join with no metrics. Every row
// carries its pod's requests and no utilisation, which is what a caller needs when
// it only cares about what is reserved (fit checks, doctor) and should not depend
// on the Metrics API being readable.
func ContainerRowsFromPods(pods []corev1.Pod) []ContainerRow {
	return buildContainerRows(pods, nil)
}

// AggregateNodes builds per-node capacity rows (sorted by name).
// Pod request/util are summed from containers scheduled on that node.
func AggregateNodes(nodes []corev1.Node, containers []ContainerRow) []NodeCapacity {
	byName := make(map[string]*NodeCapacity, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		name := n.Name
		alloc := n.Status.Allocatable
		cap := n.Status.Capacity
		byName[name] = &NodeCapacity{
			Name:           name,
			CPUAllocMcores: milliCPU(alloc.Cpu()),
			MemAllocBytes:  bytesQty(alloc.Memory()),
			CPUCapMcores:   milliCPU(cap.Cpu()),
			MemCapBytes:    bytesQty(cap.Memory()),
			Taints:         BlockingTaints(n.Spec.Taints),
			Unschedulable:  n.Spec.Unschedulable,
		}
	}
	// Requests are counted once per pod, from the pod's *effective* request
	// (max of the container sum and init containers), because that is what the
	// scheduler reserves. Summing container rows directly understates a node
	// whose pods carry heavy init containers. Utilisation still sums per
	// container: init containers produce no usage at runtime.
	type podTotals struct {
		node           *NodeCapacity
		sumCPU, sumMem float64
		podCPU, podMem float64
	}
	pods := map[string]*podTotals{}
	for _, c := range containers {
		node := strings.TrimSpace(c.Node)
		if node == "" {
			continue
		}
		row := byName[node]
		if row == nil {
			row = &NodeCapacity{Name: node}
			byName[node] = row
		}
		if c.HasUtil {
			row.CPUUtilMcores += c.CPUUtilMcores
			row.MemUtilBytes += c.MemUtilBytes
		}
		pk := c.Namespace + "/" + c.Pod
		t := pods[pk]
		if t == nil {
			t = &podTotals{node: row}
			pods[pk] = t
		}
		t.sumCPU += c.CPURequestMcores
		t.sumMem += float64(c.MemRequestBytes)
		if c.PodCPURequestMcores > t.podCPU {
			t.podCPU = c.PodCPURequestMcores
		}
		if float64(c.PodMemRequestBytes) > t.podMem {
			t.podMem = float64(c.PodMemRequestBytes)
		}
	}
	for _, t := range pods {
		// max(container sum, effective pod request). The second is authoritative
		// when the caller populated it; the sum keeps rows built by hand
		// (existing tests, other callers) behaving as before.
		cpu := t.sumCPU
		if t.podCPU > cpu {
			cpu = t.podCPU
		}
		mem := t.sumMem
		if t.podMem > mem {
			mem = t.podMem
		}
		t.node.CPUReqMcores += cpu
		t.node.MemReqBytes += int64(mem)
		t.node.Pods++
	}
	out := make([]NodeCapacity, 0, len(byName))
	for _, r := range byName {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
