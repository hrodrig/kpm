package sample

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
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

func TestRunLoopRetriesTransientThenSucceeds(t *testing.T) {
	prev := runOnceFn
	t.Cleanup(func() { runOnceFn = prev })

	var calls atomic.Int32
	runOnceFn = func(context.Context, *config.Config, *store.Store, kubernetes.Interface, metricsclientset.Interface) (Result, error) {
		n := calls.Add(1)
		if n == 1 {
			return Result{}, fmt.Errorf("metrics-server blip")
		}
		return Result{}, nil
	}

	cfg := &config.Config{Sample: config.Sample{IntervalSeconds: 1}}
	ctx, cancel := context.WithCancel(t.Context())
	var ticks atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, cfg, nil, nil, nil, func(Result) { ticks.Add(1) })
	}()

	deadline := time.After(5 * time.Second)
	for ticks.Load() < 1 {
		select {
		case <-deadline:
			cancel()
			t.Fatal("no successful tick after transient error")
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
	if calls.Load() < 2 {
		t.Fatalf("want retry, calls=%d", calls.Load())
	}
}

func TestRetryBackoff(t *testing.T) {
	interval := 30 * time.Second
	if got := retryBackoff(1, interval); got != time.Second {
		t.Fatalf("fail1: %v", got)
	}
	if got := retryBackoff(3, interval); got != 4*time.Second {
		t.Fatalf("fail3: %v", got)
	}
	if got := retryBackoff(10, 2*time.Second); got != 2*time.Second {
		t.Fatalf("capped: %v", got)
	}
}

func TestRunLoopFailsClosedAfterThreshold(t *testing.T) {
	prev := runOnceFn
	prevBudget := sampleRetryBudget
	t.Cleanup(func() {
		runOnceFn = prev
		sampleRetryBudget = prevBudget
	})
	sampleRetryBudget = 400 * time.Millisecond

	runOnceFn = func(context.Context, *config.Config, *store.Store, kubernetes.Interface, metricsclientset.Interface) (Result, error) {
		return Result{}, fmt.Errorf("persistent failure")
	}

	cfg := &config.Config{Sample: config.Sample{IntervalSeconds: 1}}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := RunLoop(ctx, cfg, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected fail-closed error")
	}
	if !strings.Contains(err.Error(), "retry budget") {
		t.Fatalf("got %v", err)
	}
}
