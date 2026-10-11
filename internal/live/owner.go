package live

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ContainerResources is request/limit for one container (template or live).
type ContainerResources struct {
	Name             string
	CPURequestMcores float64
	CPULimitMcores   float64
	MemRequestBytes  int64
	MemLimitBytes    int64
}

// OwnerTemplate is the controlling workload's pod template resources (read-only).
type OwnerTemplate struct {
	Namespace         string
	Kind              string // Deployment | StatefulSet | DaemonSet
	Name              string
	Via               string // e.g. ReplicaSet/web-abc when resolved through RS
	Containers        []ContainerResources
	Unsupported       bool // owner kind not Deploy/STS/DS
	UnsupportedReason string
}

// WorkloadGetter fetches apps controllers (narrow interface for tests).
type WorkloadGetter interface {
	GetReplicaSet(ctx context.Context, ns, name string) (*appsv1.ReplicaSet, error)
	GetDeployment(ctx context.Context, ns, name string) (*appsv1.Deployment, error)
	GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error)
	GetDaemonSet(ctx context.Context, ns, name string) (*appsv1.DaemonSet, error)
}

type k8sWorkloadGetter struct {
	cs kubernetes.Interface
}

func (g k8sWorkloadGetter) GetReplicaSet(ctx context.Context, ns, name string) (*appsv1.ReplicaSet, error) {
	return g.cs.AppsV1().ReplicaSets(ns).Get(ctx, name, metav1.GetOptions{})
}

func (g k8sWorkloadGetter) GetDeployment(ctx context.Context, ns, name string) (*appsv1.Deployment, error) {
	return g.cs.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
}

func (g k8sWorkloadGetter) GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error) {
	return g.cs.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
}

func (g k8sWorkloadGetter) GetDaemonSet(ctx context.Context, ns, name string) (*appsv1.DaemonSet, error) {
	return g.cs.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
}

// NewWorkloadGetter wraps a clientset for owner template reads.
func NewWorkloadGetter(cs kubernetes.Interface) WorkloadGetter {
	return k8sWorkloadGetter{cs: cs}
}

// ResolveOwnerTemplate loads Deploy/STS/DS pod template resources for a pod owner.
// ReplicaSet owners are walked up to Deployment. Read-only GETs only.
func ResolveOwnerTemplate(ctx context.Context, g WorkloadGetter, namespace, ownerKind, ownerName string) (OwnerTemplate, error) {
	out := OwnerTemplate{Namespace: namespace}
	if g == nil {
		return out, fmt.Errorf("live: workload getter required")
	}
	kind := ownerKind
	name := ownerName
	via := ""

	if kind == "ReplicaSet" {
		rs, err := g.GetReplicaSet(ctx, namespace, name)
		if err != nil {
			return out, err
		}
		dk, dn := controllerOwner(rs.OwnerReferences)
		if dk != "Deployment" || dn == "" {
			out.Unsupported = true
			out.UnsupportedReason = "ReplicaSet has no Deployment owner"
			out.Kind, out.Name = kind, name
			return out, nil
		}
		via = "ReplicaSet/" + name
		kind, name = dk, dn
	}

	switch kind {
	case "Deployment":
		d, err := g.GetDeployment(ctx, namespace, name)
		if err != nil {
			return out, err
		}
		out.Kind, out.Name, out.Via = kind, name, via
		out.Containers = resourcesFromPodSpec(d.Spec.Template.Spec)
		return out, nil
	case "StatefulSet":
		s, err := g.GetStatefulSet(ctx, namespace, name)
		if err != nil {
			return out, err
		}
		out.Kind, out.Name, out.Via = kind, name, via
		out.Containers = resourcesFromPodSpec(s.Spec.Template.Spec)
		return out, nil
	case "DaemonSet":
		d, err := g.GetDaemonSet(ctx, namespace, name)
		if err != nil {
			return out, err
		}
		out.Kind, out.Name, out.Via = kind, name, via
		out.Containers = resourcesFromPodSpec(d.Spec.Template.Spec)
		return out, nil
	case "":
		out.Unsupported = true
		out.UnsupportedReason = "pod has no owner"
		return out, nil
	default:
		out.Unsupported = true
		out.UnsupportedReason = "owner kind " + kind + " not Deploy/STS/DS"
		out.Kind, out.Name = kind, name
		return out, nil
	}
}

func controllerOwner(refs []metav1.OwnerReference) (kind, name string) {
	for _, o := range refs {
		if o.Controller != nil && *o.Controller {
			return o.Kind, o.Name
		}
	}
	if len(refs) > 0 {
		return refs[0].Kind, refs[0].Name
	}
	return "", ""
}

func resourcesFromPodSpec(spec corev1.PodSpec) []ContainerResources {
	out := make([]ContainerResources, 0, len(spec.Containers))
	for _, c := range spec.Containers {
		cr := ContainerResources{Name: c.Name}
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			cr.CPURequestMcores = float64(q.MilliValue())
		}
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			cr.MemRequestBytes = q.Value()
		}
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			cr.CPULimitMcores = float64(q.MilliValue())
		}
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			cr.MemLimitBytes = q.Value()
		}
		out = append(out, cr)
	}
	return out
}

// LiveContainerResources extracts live allocation rows for one pod.
func LiveContainerResources(containers []ContainerRow, namespace, pod string) []ContainerResources {
	var out []ContainerResources
	for _, c := range containers {
		if c.Namespace != namespace || c.Pod != pod {
			continue
		}
		out = append(out, ContainerResources{
			Name:             c.Container,
			CPURequestMcores: c.CPURequestMcores,
			CPULimitMcores:   c.CPULimitMcores,
			MemRequestBytes:  c.MemRequestBytes,
			MemLimitBytes:    c.MemLimitBytes,
		})
	}
	return out
}
