// Package fit answers whether a workload can be placed on a node, which is a
// different question from how much capacity the cluster holds.
package fit

import (
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// Unmodelled lists the scheduler filters this package does not evaluate. Callers
// are expected to surface it: an answer that silently ignored them would look
// authoritative and be wrong.
const Unmodelled = "pod (anti-)affinity, topology spread constraints, volume zone binding, and fragmentation across nodes"

// Pod is the part of a pod template that decides placement.
type Pod struct {
	CPURequestMcores float64
	MemRequestBytes  int64
	Tolerations      []corev1.Toleration
	NodeSelector     map[string]string
	NodeAffinity     *corev1.NodeSelector
}

// Node is one candidate node, with what is already reserved on it.
type Node struct {
	Name           string
	Labels         map[string]string
	Taints         []corev1.Taint
	Unschedulable  bool
	CPUAllocMcores float64
	MemAllocBytes  int64
	CPUReqMcores   float64
	MemReqBytes    int64
}

// Verdict is the answer for one node. Fits is true only when every evaluated
// filter passes; Reasons explains each failure in the order the scheduler would
// reject it.
type Verdict struct {
	Node    string
	Fits    bool
	Reasons []string
}

func (v *Verdict) add(reason string) {
	v.Fits = false
	v.Reasons = append(v.Reasons, reason)
}

// Evaluate reports, per node, whether the pod could be placed there.
//
// It models the filters that can be decided from the node and the pod alone:
// cordon, taints and tolerations, nodeSelector, required node affinity, and
// remaining CPU and memory. Everything in Unmodelled is out of scope by
// construction, not by oversight.
func Evaluate(pod Pod, nodes []Node) []Verdict {
	out := make([]Verdict, 0, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		v := Verdict{Node: n.Name, Fits: true}

		if n.Unschedulable {
			v.add("node is cordoned")
		}
		for _, t := range n.Taints {
			if blocks(t) && !tolerated(pod.Tolerations, t) {
				v.add("taint " + formatTaint(t) + " not tolerated")
			}
		}
		for _, reason := range selectorFailures(pod.NodeSelector, n.Labels) {
			v.add(reason)
		}
		if !affinityMatches(pod.NodeAffinity, n.Labels) {
			v.add("required node affinity not matched")
		}
		if free := n.CPUAllocMcores - n.CPUReqMcores; pod.CPURequestMcores > free {
			v.add(fmt.Sprintf("insufficient cpu: needs %s, free %s",
				FormatMilliCPU(pod.CPURequestMcores), FormatMilliCPU(free)))
		}
		if free := n.MemAllocBytes - n.MemReqBytes; pod.MemRequestBytes > free {
			v.add(fmt.Sprintf("insufficient memory: needs %s, free %s",
				FormatBytes(pod.MemRequestBytes), FormatBytes(free)))
		}
		out = append(out, v)
	}
	return out
}

// Fitting returns the names of the nodes that fit.
func Fitting(verdicts []Verdict) []string {
	var out []string
	for _, v := range verdicts {
		if v.Fits {
			out = append(out, v.Node)
		}
	}
	return out
}

func blocks(t corev1.Taint) bool {
	return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
}

func formatTaint(t corev1.Taint) string {
	s := t.Key
	if t.Value != "" {
		s += "=" + t.Value
	}
	return s + ":" + string(t.Effect)
}

// tolerated follows the scheduler: an empty effect toleration matches any effect,
// the Exists operator with an empty key matches every taint, and an empty
// operator is Equal.
func tolerated(tolerations []corev1.Toleration, t corev1.Taint) bool {
	for _, tol := range tolerations {
		if tol.Effect != "" && tol.Effect != t.Effect {
			continue
		}
		if tol.Operator == corev1.TolerationOpExists {
			if tol.Key == "" || tol.Key == t.Key {
				return true
			}
			continue
		}
		if tol.Key == t.Key && tol.Value == t.Value {
			return true
		}
	}
	return false
}

// selectorFailures checks nodeSelector. Keys are sorted so the reasons come out
// stable: map iteration order would make the output differ between runs.
func selectorFailures(selector, labels map[string]string) []string {
	if len(selector) == 0 {
		return nil
	}
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []string
	for _, k := range keys {
		if got, ok := labels[k]; !ok || got != selector[k] {
			out = append(out, fmt.Sprintf("nodeSelector %s=%s not matched", k, selector[k]))
		}
	}
	return out
}

// affinityMatches evaluates required node affinity: the terms are ORed and the
// expressions within a term are ANDed, as the scheduler does. An empty selector
// matches everything.
func affinityMatches(sel *corev1.NodeSelector, labels map[string]string) bool {
	if sel == nil || len(sel.NodeSelectorTerms) == 0 {
		return true
	}
	for _, term := range sel.NodeSelectorTerms {
		ok := true
		for _, expr := range term.MatchExpressions {
			if !matchExpr(expr, labels) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func matchExpr(e corev1.NodeSelectorRequirement, labels map[string]string) bool {
	got, has := labels[e.Key]
	switch e.Operator {
	case corev1.NodeSelectorOpIn:
		return has && contains(e.Values, got)
	case corev1.NodeSelectorOpNotIn:
		return !has || !contains(e.Values, got)
	case corev1.NodeSelectorOpExists:
		return has
	case corev1.NodeSelectorOpDoesNotExist:
		return !has
	case corev1.NodeSelectorOpGt, corev1.NodeSelectorOpLt:
		return compareInts(e, got, has)
	}
	return false
}

// compareInts handles the Gt/Lt operators, whose value must be an integer on both
// sides. A non-integer label never matches rather than being coerced.
func compareInts(e corev1.NodeSelectorRequirement, got string, has bool) bool {
	if !has || len(e.Values) != 1 {
		return false
	}
	want, err := strconv.Atoi(e.Values[0])
	if err != nil {
		return false
	}
	have, err := strconv.Atoi(got)
	if err != nil {
		return false
	}
	if e.Operator == corev1.NodeSelectorOpGt {
		return have > want
	}
	return have < want
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// FormatMilliCPU renders a millicore count for humans.
func FormatMilliCPU(m float64) string {
	return strconv.FormatFloat(m, 'f', -1, 64) + "m"
}

// FormatBytes renders a byte count in the largest binary unit that fits.
func FormatBytes(b int64) string {
	const (
		ki = int64(1024)
		mi = ki * 1024
		gi = mi * 1024
	)
	switch {
	case b >= gi:
		return strconv.FormatFloat(float64(b)/float64(gi), 'f', 1, 64) + "Gi"
	case b >= mi:
		return strconv.FormatFloat(float64(b)/float64(mi), 'f', 0, 64) + "Mi"
	default:
		return strconv.FormatInt(b, 10) + "B"
	}
}
