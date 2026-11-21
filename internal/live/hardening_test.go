package live

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
