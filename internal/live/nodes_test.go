package live

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeNodes struct {
	items []corev1.Node
	err   error
}

func (f fakeNodes) List(context.Context, metav1.ListOptions) (*corev1.NodeList, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &corev1.NodeList{Items: f.items}, nil
}

func TestAggregateNodesSumsPods(t *testing.T) {
	cpu := resource.MustParse("2")
	mem := resource.MustParse("4Gi")
	nodes := []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: mem},
			Capacity:    corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: mem},
		},
	}}
	rows := AggregateNodes(nodes, []ContainerRow{
		{Namespace: "a", Pod: "p1", Node: "n1", CPURequestMcores: 100, MemRequestBytes: 1000, HasUtil: true, CPUUtilMcores: 40, MemUtilBytes: 400},
		{Namespace: "a", Pod: "p1", Node: "n1", CPURequestMcores: 50, MemRequestBytes: 500, HasUtil: true, CPUUtilMcores: 10, MemUtilBytes: 100},
		{Namespace: "a", Pod: "p2", Node: "n1", CPURequestMcores: 200, MemRequestBytes: 2000},
	})
	if len(rows) != 1 {
		t.Fatalf("len %d", len(rows))
	}
	r := rows[0]
	if r.CPUAllocMcores != 2000 {
		t.Fatalf("alloc cpu %v", r.CPUAllocMcores)
	}
	if r.CPUReqMcores != 350 || r.MemReqBytes != 3500 {
		t.Fatalf("req %+v", r)
	}
	if r.CPUUtilMcores != 50 || r.MemUtilBytes != 500 {
		t.Fatalf("util %+v", r)
	}
	if r.Pods != 2 {
		t.Fatalf("pods %d", r.Pods)
	}
}

func TestCaptureNodeCapacity(t *testing.T) {
	cpu := resource.MustParse("1")
	f := fakeNodes{items: []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: cpu},
			Capacity:    corev1.ResourceList{corev1.ResourceCPU: cpu},
		},
	}}}
	got, err := CaptureNodeCapacity(context.Background(), f, []ContainerRow{
		{Namespace: "ns", Pod: "p", Node: "worker", CPURequestMcores: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "worker" || got[0].CPUReqMcores != 100 {
		t.Fatalf("%+v", got)
	}
}
