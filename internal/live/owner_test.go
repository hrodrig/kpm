package live

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeWorkloads struct {
	rs  *appsv1.ReplicaSet
	dep *appsv1.Deployment
	sts *appsv1.StatefulSet
	ds  *appsv1.DaemonSet
}

func (f fakeWorkloads) GetReplicaSet(context.Context, string, string) (*appsv1.ReplicaSet, error) {
	return f.rs, nil
}
func (f fakeWorkloads) GetDeployment(context.Context, string, string) (*appsv1.Deployment, error) {
	return f.dep, nil
}
func (f fakeWorkloads) GetStatefulSet(context.Context, string, string) (*appsv1.StatefulSet, error) {
	return f.sts, nil
}
func (f fakeWorkloads) GetDaemonSet(context.Context, string, string) (*appsv1.DaemonSet, error) {
	return f.ds, nil
}

func TestResolveOwnerTemplateViaReplicaSet(t *testing.T) {
	ctrl := true
	cpu := resource.MustParse("100m")
	mem := resource.MustParse("64Mi")
	f := fakeWorkloads{
		rs: &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-abc", Namespace: "app",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "Deployment", Name: "web", Controller: &ctrl,
				}},
			},
		},
		dep: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "c",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    cpu,
									corev1.ResourceMemory: mem,
								},
							},
						}},
					},
				},
			},
		},
	}
	got, err := ResolveOwnerTemplate(context.Background(), f, "app", "ReplicaSet", "web-abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "Deployment" || got.Name != "web" || got.Via != "ReplicaSet/web-abc" {
		t.Fatalf("owner: %+v", got)
	}
	if len(got.Containers) != 1 || got.Containers[0].CPURequestMcores != 100 {
		t.Fatalf("containers: %+v", got.Containers)
	}
}

func TestResolveOwnerTemplateStatefulSet(t *testing.T) {
	cpu := resource.MustParse("200m")
	f := fakeWorkloads{
		sts: &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "app"},
			Spec: appsv1.StatefulSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "pg",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceCPU: cpu},
							},
						}},
					},
				},
			},
		},
	}
	got, err := ResolveOwnerTemplate(context.Background(), f, "app", "StatefulSet", "db")
	if err != nil || got.Kind != "StatefulSet" || got.Containers[0].CPURequestMcores != 200 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestResolveOwnerUnsupported(t *testing.T) {
	got, err := ResolveOwnerTemplate(context.Background(), fakeWorkloads{}, "app", "Job", "j")
	if err != nil || !got.Unsupported {
		t.Fatalf("%+v %v", got, err)
	}
	got, err = ResolveOwnerTemplate(context.Background(), fakeWorkloads{}, "app", "", "")
	if err != nil || !got.Unsupported {
		t.Fatalf("empty owner: %+v %v", got, err)
	}
}

func TestResolveOwnerDaemonSet(t *testing.T) {
	cpu := resource.MustParse("50m")
	f := fakeWorkloads{
		ds: &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "app"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "a",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceCPU: cpu},
							},
						}},
					},
				},
			},
		},
	}
	got, err := ResolveOwnerTemplate(context.Background(), f, "app", "DaemonSet", "agent")
	if err != nil || got.Kind != "DaemonSet" || got.Containers[0].CPURequestMcores != 50 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestLiveContainerResources(t *testing.T) {
	rows := []ContainerRow{
		{Namespace: "a", Pod: "p", Container: "c", CPURequestMcores: 50, MemRequestBytes: 1 << 20},
		{Namespace: "a", Pod: "other", Container: "c", CPURequestMcores: 1},
	}
	got := LiveContainerResources(rows, "a", "p")
	if len(got) != 1 || got[0].CPURequestMcores != 50 {
		t.Fatalf("%+v", got)
	}
}
