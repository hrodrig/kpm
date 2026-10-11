package sample

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/hrodrig/kpm/internal/config"
)

type fakePods struct {
	items []corev1.Pod
	err   error
}

func (f fakePods) List(_ context.Context, _ string, _ metav1.ListOptions) (*corev1.PodList, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &corev1.PodList{Items: f.items}, nil
}

type fakeMetrics struct {
	items []metricsv1beta1.PodMetrics
	err   error
}

func (f fakeMetrics) List(_ context.Context, _ string, _ metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &metricsv1beta1.PodMetricsList{Items: f.items}, nil
}

func TestCaptureHappyPath(t *testing.T) {
	at := time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC)
	pods := fakePods{items: []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				},
			}},
		},
	}}}
	metrics := fakeMetrics{items: []metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name:  "c",
			Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("40m")},
		}},
	}}}

	res, err := Capture(t.Context(), CaptureOptions{
		Pods:    pods,
		Metrics: metrics,
		Scope:   config.Scope{Namespaces: []string{"app"}},
		Now:     func() time.Time { return at },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("records: %d", len(res.Records))
	}
	if res.Records[0].CPUUtilMcores != 40 {
		t.Fatalf("util: %v", res.Records[0].CPUUtilMcores)
	}
}

func TestCaptureRequiresListers(t *testing.T) {
	if _, err := Capture(t.Context(), CaptureOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCapturePodListError(t *testing.T) {
	_, err := Capture(t.Context(), CaptureOptions{
		Pods:    fakePods{err: fmt.Errorf("denied")},
		Metrics: fakeMetrics{},
	})
	if err == nil {
		t.Fatal("expected list pods error")
	}
}

func TestCaptureMetricsError(t *testing.T) {
	_, err := Capture(t.Context(), CaptureOptions{
		Pods:    fakePods{},
		Metrics: fakeMetrics{err: fmt.Errorf("no metrics")},
	})
	if err == nil {
		t.Fatal("expected metrics error")
	}
}
