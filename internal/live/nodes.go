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
		}
	}
	seenPods := map[string]bool{}
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
		row.CPUReqMcores += c.CPURequestMcores
		row.MemReqBytes += c.MemRequestBytes
		if c.HasUtil {
			row.CPUUtilMcores += c.CPUUtilMcores
			row.MemUtilBytes += c.MemUtilBytes
		}
		pk := c.Namespace + "/" + c.Pod
		if !seenPods[pk] {
			seenPods[pk] = true
			row.Pods++
		}
	}
	out := make([]NodeCapacity, 0, len(byName))
	for _, r := range byName {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
