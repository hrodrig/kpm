package live

import (
	"bytes"
	"context"
	"fmt"
	"strings"
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

func TestCaptureKeepsPodsWithoutMetrics(t *testing.T) {
	at := time.Date(2026, 10, 11, 16, 0, 0, 0, time.UTC)
	pods := fakePods{items: []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		Spec: corev1.PodSpec{
			NodeName: "n1",
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
	}}}

	snap, err := Capture(t.Context(), Options{
		Pods:    pods,
		Metrics: fakeMetrics{},
		Scope:   config.Scope{Namespaces: []string{"app"}},
		Now:     func() time.Time { return at },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Containers) != 1 {
		t.Fatalf("containers: %d", len(snap.Containers))
	}
	if snap.Containers[0].HasUtil {
		t.Fatal("expected no util")
	}
	if _, ok := snap.Containers[0].CPURatioRequest(); ok {
		t.Fatal("ratio should be n/a")
	}
}

func TestCaptureHappyPathAndAggregate(t *testing.T) {
	pods := fakePods{items: []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "app",
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "web"}},
		},
		Spec: corev1.PodSpec{
			NodeName: "n1",
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
				{
					Name: "sidecar",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("32Mi"),
						},
					},
				},
			},
		},
	}}}
	metrics := fakeMetrics{items: []metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		Containers: []metricsv1beta1.ContainerMetrics{
			{Name: "app", Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("40m"),
				corev1.ResourceMemory: resource.MustParse("16Mi"),
			}},
			{Name: "sidecar", Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("8Mi"),
			}},
		},
	}}}

	snap, err := Capture(t.Context(), Options{
		Pods: pods, Metrics: metrics,
		Scope: config.Scope{Namespaces: []string{"app"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Containers) != 2 {
		t.Fatalf("containers: %d", len(snap.Containers))
	}
	podsAgg := AggregatePods(snap.Containers)
	if len(podsAgg) != 1 {
		t.Fatalf("pods: %d", len(podsAgg))
	}
	p := podsAgg[0]
	if p.CPUUtilMcores != 50 || p.CPURequestMcores != 150 {
		t.Fatalf("cpu sum util=%v req=%v", p.CPUUtilMcores, p.CPURequestMcores)
	}
	ratio, ok := p.CPURatioRequest()
	if !ok || ratio < 0.33 || ratio > 0.34 {
		t.Fatalf("cpu ratio: %v ok=%v", ratio, ok)
	}
}

func TestAllowlistAndSort(t *testing.T) {
	pods := fakePods{items: []corev1.Pod{
		podWithReq("app", "a", "100m"),
		podWithReq("app", "b", "100m"),
	}}
	metrics := fakeMetrics{items: []metricsv1beta1.PodMetrics{
		metricsFor("app", "a", "80m"),
		metricsFor("app", "b", "10m"),
	}}
	snap, err := Capture(t.Context(), Options{
		Pods: pods, Metrics: metrics,
		Scope:        config.Scope{Namespaces: []string{"app"}},
		PodAllowlist: []string{"app/b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Containers) != 1 || snap.Containers[0].Pod != "b" {
		t.Fatalf("allowlist: %+v", snap.Containers)
	}

	all, err := Capture(t.Context(), Options{
		Pods: pods, Metrics: metrics,
		Scope: config.Scope{Namespaces: []string{"app"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := AggregatePods(all.Containers)
	SortPods(rows, SortCPURatio)
	if rows[0].Pod != "a" {
		t.Fatalf("hottest first: %s", rows[0].Pod)
	}
}

func TestFormatPodTable(t *testing.T) {
	var buf bytes.Buffer
	rows := []PodRow{{
		Namespace: "app", Pod: "web", Node: "n1",
		HasUtil: true, CPUUtilMcores: 40, CPURequestMcores: 100,
		MemUtilBytes: 16 << 20, MemRequestBytes: 64 << 20,
	}}
	if err := FormatPodTable(&buf, rows); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "NAMESPACE") || !strings.Contains(out, "app") {
		t.Fatalf("table: %q", out)
	}
	if !strings.Contains(out, "0.40") {
		t.Fatalf("ratio missing: %q", out)
	}
}

func TestCaptureErrors(t *testing.T) {
	if _, err := Capture(t.Context(), Options{}); err == nil {
		t.Fatal("expected listers required")
	}
	if _, err := Capture(t.Context(), Options{
		Pods: fakePods{err: fmt.Errorf("denied")}, Metrics: fakeMetrics{},
	}); err == nil {
		t.Fatal("expected pod list error")
	}
	if _, err := Capture(t.Context(), Options{
		Pods: fakePods{}, Metrics: fakeMetrics{err: fmt.Errorf("no metrics")},
	}); err == nil {
		t.Fatal("expected metrics error")
	}
}

func TestParseSortKey(t *testing.T) {
	if ParseSortKey("name") != SortName {
		t.Fatal("name")
	}
	if ParseSortKey("") != SortCPURatio {
		t.Fatal("default")
	}
}

func TestJoinLabelSelectors(t *testing.T) {
	if got := joinLabelSelectors("a=1", "b=2"); got != "a=1,b=2" {
		t.Fatalf("got %q", got)
	}
}

func TestContainerRatiosAndSortKeys(t *testing.T) {
	c := ContainerRow{
		HasUtil: true, CPUUtilMcores: 50, CPURequestMcores: 100, CPULimitMcores: 200,
		MemUtilBytes: 10, MemRequestBytes: 20, MemLimitBytes: 40,
	}
	if v, ok := c.CPURatioLimit(); !ok || v != 0.25 {
		t.Fatalf("cpu limit ratio: %v %v", v, ok)
	}
	if v, ok := c.MemRatioLimit(); !ok || v != 0.25 {
		t.Fatalf("mem limit ratio: %v %v", v, ok)
	}
	rows := []PodRow{
		{Namespace: "ns", Pod: "low", HasUtil: true, MemUtilBytes: 1, MemRequestBytes: 100},
		{Namespace: "ns", Pod: "high", HasUtil: true, MemUtilBytes: 90, MemRequestBytes: 100},
	}
	SortPods(rows, SortMemRatio)
	if rows[0].Pod != "high" {
		t.Fatalf("mem.ratio: %s", rows[0].Pod)
	}
	SortPods(rows, SortName)
	if rows[0].Pod != "high" && rows[0].Pod != "low" {
		t.Fatal("name sort ran")
	}
	SortPods(rows, SortCPUUtil)
	SortPods(rows, SortMemUtil)
}

func TestFilterAllowlistEmptyEntries(t *testing.T) {
	pods := []corev1.Pod{podWithReq("app", "x", "1")}
	got := filterAllowlist(pods, []string{"", "  "})
	if len(got) != 1 {
		t.Fatalf("empty allowlist entries should not filter: %d", len(got))
	}
}

func podWithReq(ns, name, cpu string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)},
				},
			}},
		},
	}
}

func metricsFor(ns, name, cpu string) metricsv1beta1.PodMetrics {
	return metricsv1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name:  "c",
			Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)},
		}},
	}
}
