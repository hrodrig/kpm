package sample

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func TestRunLoopStopsOnCancel(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				},
			}},
		},
	}
	pm := &metricsv1beta1.PodMetrics{
		TypeMeta:   metav1.TypeMeta{Kind: "PodMetrics", APIVersion: "metrics.k8s.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name:  "c",
			Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m")},
		}},
	}
	cs := fake.NewSimpleClientset(pod)
	mcs := metricsfake.NewSimpleClientset()
	if err := mcs.Tracker().Create(metricsv1beta1.SchemeGroupVersion.WithResource("pods"), pm, "app"); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(t.TempDir() + "/m.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{
		Cluster: config.Cluster{Name: "c"},
		Client:  config.Client{ID: "loop-test"},
		Store:   config.Store{RetentionDays: 14},
		Sample:  config.Sample{IntervalSeconds: 1},
		Scope:   config.Scope{Namespaces: []string{"app"}},
	}

	ctx, cancel := context.WithCancel(t.Context())
	var ticks atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, cfg, st, cs, mcs, func(Result) { ticks.Add(1) })
	}()

	deadline := time.After(3 * time.Second)
	for ticks.Load() < 1 {
		select {
		case <-deadline:
			cancel()
			t.Fatal("no tick before deadline")
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunLoop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunLoop did not stop")
	}
}

func TestRunLoopRejectsZeroInterval(t *testing.T) {
	err := RunLoop(t.Context(), &config.Config{Sample: config.Sample{IntervalSeconds: 0}}, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
