package sample

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/identity"
	"github.com/hrodrig/kpm/internal/store"
)

// PodLister lists pods (narrow interface for tests).
type PodLister interface {
	List(ctx context.Context, namespace string, opts metav1.ListOptions) (*corev1.PodList, error)
}

// MetricsLister lists pod metrics.
type MetricsLister interface {
	List(ctx context.Context, namespace string, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error)
}

// k8sPodLister adapts the typed clientset.
type k8sPodLister struct {
	cs kubernetes.Interface
}

func (l k8sPodLister) List(ctx context.Context, namespace string, opts metav1.ListOptions) (*corev1.PodList, error) {
	return l.cs.CoreV1().Pods(namespace).List(ctx, opts)
}

type k8sMetricsLister struct {
	cs metricsclientset.Interface
}

func (l k8sMetricsLister) List(ctx context.Context, namespace string, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error) {
	return l.cs.MetricsV1beta1().PodMetricses(namespace).List(ctx, opts)
}

// CaptureOptions configures one sample pass.
type CaptureOptions struct {
	Pods    PodLister
	Metrics MetricsLister
	Scope   config.Scope
	Now     func() time.Time
}

// Capture lists pods + metrics for scope and builds records.
func Capture(ctx context.Context, opt CaptureOptions) (Result, error) {
	if opt.Pods == nil || opt.Metrics == nil {
		return Result{}, fmt.Errorf("sample: pods and metrics listers required")
	}
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}

	namespaces := opt.Scope.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	listOpts := metav1.ListOptions{}
	if s := opt.Scope.PodLabels; s != "" {
		listOpts.LabelSelector = s
	}

	var pods []corev1.Pod
	var metrics []metricsv1beta1.PodMetrics
	for _, ns := range namespaces {
		pl, err := opt.Pods.List(ctx, ns, listOpts)
		if err != nil {
			return Result{}, fmt.Errorf("list pods: %w", err)
		}
		pods = append(pods, pl.Items...)

		ml, err := opt.Metrics.List(ctx, ns, listOpts)
		if err != nil {
			return Result{}, fmt.Errorf("list pod metrics: %w", err)
		}
		metrics = append(metrics, ml.Items...)
	}

	return BuildRecords(now(), pods, metrics), nil
}

// RunOnce opens the store (caller-provided), captures, inserts, and prunes.
func RunOnce(ctx context.Context, cfg *config.Config, st *store.Store, pods kubernetes.Interface, metrics metricsclientset.Interface) (Result, error) {
	res, err := Capture(ctx, CaptureOptions{
		Pods:    k8sPodLister{cs: pods},
		Metrics: k8sMetricsLister{cs: metrics},
		Scope:   cfg.Scope,
	})
	if err != nil {
		return Result{}, err
	}
	clientID := identity.Resolve(cfg.Client.ID)
	clusterName := cfg.Cluster.Name
	for i := range res.Records {
		res.Records[i].ClientID = clientID
		res.Records[i].ClusterName = clusterName
	}
	t0 := time.Now()
	if err := st.InsertRecords(ctx, res.Records); err != nil {
		return Result{}, fmt.Errorf("insert samples: %w", err)
	}
	res.StoreWrite = time.Since(t0)
	if _, err := st.PruneRetention(ctx, cfg.Store.RetentionDays); err != nil {
		return Result{}, fmt.Errorf("prune retention: %w", err)
	}
	return res, nil
}
