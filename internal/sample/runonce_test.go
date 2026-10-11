package sample

import (
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func TestRunOncePersistsIdentity(t *testing.T) {
	t.Setenv("KPM_CLIENT_ID", "bastion-test-01")
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodMetrics",
			APIVersion: "metrics.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name:  "c",
			Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("25m")},
		}},
	}
	cs := fake.NewSimpleClientset(pod)
	mcs := metricsfake.NewSimpleClientset()
	if err := mcs.Tracker().Create(
		metricsv1beta1.SchemeGroupVersion.WithResource("pods"),
		pm, "app",
	); err != nil {
		t.Fatalf("seed metrics: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "m.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{
		Cluster: config.Cluster{Name: "demo"},
		Client:  config.Client{ID: "ignored-when-env-set"},
		Store:   config.Store{Path: dbPath, RetentionDays: 14},
		Scope:   config.Scope{Namespaces: []string{"app"}},
	}
	res, err := RunOnce(t.Context(), cfg, st, cs, mcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("records: %d", len(res.Records))
	}
	if res.Records[0].ClientID != "bastion-test-01" {
		t.Fatalf("client_id: %q", res.Records[0].ClientID)
	}
	if res.Records[0].ClusterName != "demo" {
		t.Fatalf("cluster: %q", res.Records[0].ClusterName)
	}
	n, err := st.Count(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}
