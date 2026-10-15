package sample

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hrodrig/kpm/internal/store"
)

type fakeOwners struct {
	rs  *appsv1.ReplicaSet
	dep *appsv1.Deployment
}

func (f fakeOwners) GetReplicaSet(context.Context, string, string) (*appsv1.ReplicaSet, error) {
	return f.rs, nil
}
func (f fakeOwners) GetDeployment(context.Context, string, string) (*appsv1.Deployment, error) {
	return f.dep, nil
}
func (f fakeOwners) GetStatefulSet(context.Context, string, string) (*appsv1.StatefulSet, error) {
	return nil, nil
}
func (f fakeOwners) GetDaemonSet(context.Context, string, string) (*appsv1.DaemonSet, error) {
	return nil, nil
}

func TestResolveOwnersMapsReplicaSetToDeployment(t *testing.T) {
	ctrl := true
	g := fakeOwners{
		rs: &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-7d9f8c", Namespace: "app",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "Deployment", Name: "web", Controller: &ctrl,
				}},
			},
		},
		dep: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		},
	}
	recs := []store.Record{
		{Namespace: "app", OwnerKind: "ReplicaSet", OwnerName: "web-7d9f8c"},
		{Namespace: "app", OwnerKind: "ReplicaSet", OwnerName: "web-7d9f8c"},
		{Namespace: "app", OwnerKind: "StatefulSet", OwnerName: "db"},
	}
	ResolveOwners(context.Background(), g, recs)
	if recs[0].OwnerKind != "Deployment" || recs[0].OwnerName != "web" {
		t.Fatalf("first: %+v", recs[0])
	}
	if recs[1].OwnerKind != "Deployment" || recs[1].OwnerName != "web" {
		t.Fatalf("cached: %+v", recs[1])
	}
	if recs[2].OwnerKind != "StatefulSet" || recs[2].OwnerName != "db" {
		t.Fatalf("sts untouched: %+v", recs[2])
	}
}

func TestResolveOwnersNilSafe(t *testing.T) {
	ResolveOwners(context.Background(), nil, []store.Record{{OwnerKind: "ReplicaSet"}})
	ResolveOwners(context.Background(), fakeOwners{}, nil)
}
