package live

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hrodrig/kpm/internal/config"
)

func TestOwnerRefPrefersController(t *testing.T) {
	f, tr := false, true
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "SidecarInjector", Name: "mesh", Controller: &f},
				{Kind: "StatefulSet", Name: "db", Controller: &tr},
			},
		},
	}
	kind, name := ownerRef(p)
	if kind != "StatefulSet" || name != "db" {
		t.Fatalf("want StatefulSet/db, got %s/%s", kind, name)
	}
}

func TestNilClientsDoNotPanic(t *testing.T) {
	if _, err := CaptureClients(context.Background(), nil, nil, Options{}); err == nil {
		t.Fatal("CaptureClients with nil clients must return error")
	}
	if _, err := CaptureNodeCapacityClients(context.Background(), nil, nil); err == nil {
		t.Fatal("CaptureNodeCapacityClients with nil client must return error")
	}
	if g := NewWorkloadGetter(nil); g != nil {
		t.Fatalf("NewWorkloadGetter(nil) must return nil, got %#v", g)
	}
}

func TestNormalizeNamespaces(t *testing.T) {
	if got := NormalizeNamespaces(nil); len(got) != 1 || got[0] != "" {
		t.Fatalf("nil: %v", got)
	}
	if got := NormalizeNamespaces([]string{"app", "app", " ops ", "app"}); len(got) != 2 || got[0] != "app" || got[1] != "ops" {
		t.Fatalf("dedupe: %v", got)
	}
	if got := NormalizeNamespaces([]string{"app", ""}); len(got) != 1 || got[0] != "" {
		t.Fatalf("empty entry must collapse to NamespaceAll: %v", got)
	}
}

type pagedPods struct {
	pages map[string]*corev1.PodList
	calls int
}

func (p *pagedPods) List(_ context.Context, _ string, opts metav1.ListOptions) (*corev1.PodList, error) {
	p.calls++
	return p.pages[opts.Continue], nil
}

type pagedNodes struct {
	pages map[string]*corev1.NodeList
	calls int
}

func (p *pagedNodes) List(_ context.Context, opts metav1.ListOptions) (*corev1.NodeList, error) {
	p.calls++
	return p.pages[opts.Continue], nil
}

func TestCaptureAndNodesPaginateAndDeduplicateNamespaces(t *testing.T) {
	pods := &pagedPods{pages: map[string]*corev1.PodList{
		"": {
			ListMeta: metav1.ListMeta{Continue: "tok1"},
			Items:    []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "p1"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}}},
		},
		"tok1": {
			Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "p2"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}}},
		},
	}}
	snap, err := Capture(context.Background(), Options{
		Pods:    pods,
		Metrics: fakeMetrics{},
		Scope:   config.Scope{Namespaces: []string{"app", "app"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pods.calls != 2 || len(snap.Containers) != 2 {
		t.Fatalf("expected 2 paginated calls and 2 containers (deduped ns), calls=%d rows=%d", pods.calls, len(snap.Containers))
	}

	nodes := &pagedNodes{pages: map[string]*corev1.NodeList{
		"":   {ListMeta: metav1.ListMeta{Continue: "n1"}, Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}}},
		"n1": {Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}}}},
	}}
	gotNodes, err := CaptureNodeCapacity(context.Background(), nodes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if nodes.calls != 2 || len(gotNodes) != 2 {
		t.Fatalf("expected 2 paginated node calls and 2 nodes, calls=%d nodes=%d", nodes.calls, len(gotNodes))
	}
}
