package sample

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/hrodrig/kpm/internal/config"
)

func TestBuildRecordsSkipsContainersMissingFromMetrics(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	pods := []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "web-0"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				}},
				{Name: "restarting-sidecar", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
				}},
			},
		},
	}}
	metrics := []metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "web-0"},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name:  "app",
			Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("40m")},
		}},
	}}

	res := BuildRecords(now, pods, metrics)
	if len(res.Records) != 1 || res.Records[0].Container != "app" {
		t.Fatalf("missing container must not be recorded as synthetic 0: %+v", res.Records)
	}
}

func TestOwnerRefPrefersController(t *testing.T) {
	f, tr := false, true
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "CustomOwner", Name: "non-ctrl", Controller: &f},
				{Kind: "ReplicaSet", Name: "web-abc12", Controller: &tr},
			},
		},
	}
	kind, name := ownerRef(p)
	if kind != "ReplicaSet" || name != "web-abc12" {
		t.Fatalf("want controller ReplicaSet/web-abc12, got %s/%s", kind, name)
	}

	// Falls back to first ref when no ref sets Controller: true.
	p2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{Kind: "Job", Name: "batch-1"}},
		},
	}
	kind2, name2 := ownerRef(p2)
	if kind2 != "Job" || name2 != "batch-1" {
		t.Fatalf("fallback ownerRef: got %s/%s", kind2, name2)
	}
}

func TestSetFailHookConcurrentSafe(t *testing.T) {
	prev := SetFailHook(nil)
	t.Cleanup(func() { SetFailHook(prev) })

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				SetFailHook(func(int, error) {})
				SetFailHook(nil)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				callFailHook(j, errors.New("transient"))
			}
		}()
	}
	wg.Wait()
}

func TestRunOnceRejectsNilClientsWithoutPanic(t *testing.T) {
	cfg := &config.Config{}
	if _, err := RunOnce(context.Background(), cfg, nil, nil, nil); err == nil {
		t.Fatal("expected error for nil clientsets, not panic")
	}
}

type pagedSamplePods struct {
	pages map[string]*corev1.PodList
	calls int
}

func (p *pagedSamplePods) List(_ context.Context, _ string, opts metav1.ListOptions) (*corev1.PodList, error) {
	p.calls++
	return p.pages[opts.Continue], nil
}

func TestCapturePaginatesAndDeduplicatesNamespaces(t *testing.T) {
	pods := &pagedSamplePods{pages: map[string]*corev1.PodList{
		"": {
			ListMeta: metav1.ListMeta{Continue: "c1"},
			Items:    []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "p1"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}}},
		},
		"c1": {
			Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "p2"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}}},
		},
	}}
	metrics := fakeMetrics{items: []metricsv1beta1.PodMetrics{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "p1"}, Containers: []metricsv1beta1.ContainerMetrics{{Name: "c"}}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "p2"}, Containers: []metricsv1beta1.ContainerMetrics{{Name: "c"}}},
	}}

	res, err := Capture(context.Background(), CaptureOptions{
		Pods:    pods,
		Metrics: metrics,
		Scope:   config.Scope{Namespaces: []string{"app", "app"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pods.calls != 2 || len(res.Records) != 2 {
		t.Fatalf("expected 2 paginated calls and 2 records (deduped ns), calls=%d recs=%d", pods.calls, len(res.Records))
	}
}
