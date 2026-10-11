package sample

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestBuildRecordsOwnerAndLimits(t *testing.T) {
	at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	trueVal := true
	pods := []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       "ReplicaSet",
				Name:       "a-rs",
				Controller: &trueVal,
			}},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			}},
		},
	}}
	metrics := []metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name: "c",
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		}},
	}}
	res := BuildRecords(at, pods, metrics)
	if len(res.Records) != 1 {
		t.Fatalf("records: %d", len(res.Records))
	}
	r := res.Records[0]
	if r.OwnerKind != "ReplicaSet" || r.OwnerName != "a-rs" {
		t.Fatalf("owner: %s/%s", r.OwnerKind, r.OwnerName)
	}
	if r.CPULimitMcores != 200 || r.Node != "node-1" {
		t.Fatalf("limits/node: cpu=%v node=%q", r.CPULimitMcores, r.Node)
	}
}

func TestBuildRecordsSkipsMissingMetrics(t *testing.T) {
	at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "c",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c"}},
			},
		},
	}
	metrics := []metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name: "c",
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		}},
	}}

	res := BuildRecords(at, pods, metrics)
	if res.SkippedPods != 1 {
		t.Fatalf("skipped: got %d", res.SkippedPods)
	}
	if len(res.Records) != 1 {
		t.Fatalf("records: got %d", len(res.Records))
	}
	r := res.Records[0]
	if r.CPUUtilMcores != 50 {
		t.Fatalf("cpu util mcores: got %v", r.CPUUtilMcores)
	}
	if r.CPURequestMcores != 100 {
		t.Fatalf("cpu request mcores: got %v", r.CPURequestMcores)
	}
}
