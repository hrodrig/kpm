package tui

import (
	"fmt"
	"strings"

	"github.com/hrodrig/kpm/internal/live"
)

// FormatOwnerDetail builds a read-only compare panel: owner template vs pod live.
func FormatOwnerDetail(ns, pod string, tmpl live.OwnerTemplate, liveRows []live.ContainerResources) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("pod %s/%s\n", ns, pod))
	if tmpl.Unsupported {
		b.WriteString(fmt.Sprintf("owner template: n/a (%s)\n", tmpl.UnsupportedReason))
		b.WriteString("\n[esc] close\n")
		return b.String()
	}
	via := ""
	if tmpl.Via != "" {
		via = " via " + tmpl.Via
	}
	b.WriteString(fmt.Sprintf("owner %s/%s%s (template — read-only GET)\n\n", tmpl.Kind, tmpl.Name, via))
	b.WriteString("CONTAINER          TEMPLATE_REQ     TEMPLATE_LIM     POD_LIVE_REQ     POD_LIVE_LIM\n")

	liveBy := map[string]live.ContainerResources{}
	for _, c := range liveRows {
		liveBy[c.Name] = c
	}
	names := orderedNames(tmpl.Containers, liveRows)
	for _, name := range names {
		t := findRes(tmpl.Containers, name)
		l := liveBy[name]
		b.WriteString(fmt.Sprintf("%-18s %-16s %-16s %-16s %s\n",
			trunc(name, 18),
			cpuMemPair(t.CPURequestMcores, t.MemRequestBytes),
			cpuMemPair(t.CPULimitMcores, t.MemLimitBytes),
			cpuMemPair(l.CPURequestMcores, l.MemRequestBytes),
			cpuMemPair(l.CPULimitMcores, l.MemLimitBytes),
		))
		if t.Name != "" && (t.CPURequestMcores != l.CPURequestMcores || t.MemRequestBytes != l.MemRequestBytes ||
			t.CPULimitMcores != l.CPULimitMcores || t.MemLimitBytes != l.MemLimitBytes) {
			b.WriteString("                   ^ template ≠ live pod (mutation/VPA/manual?)\n")
		}
	}
	b.WriteString("\n[esc] close   (no cluster writes)\n")
	return b.String()
}

func cpuMemPair(cpu float64, mem int64) string {
	return live.FormatMillicores(cpu) + "/" + live.FormatBytes(mem)
}

func findRes(list []live.ContainerResources, name string) live.ContainerResources {
	for _, c := range list {
		if c.Name == name {
			return c
		}
	}
	return live.ContainerResources{Name: name}
}

func orderedNames(tmpl, liveRows []live.ContainerResources) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, c := range tmpl {
		if _, ok := seen[c.Name]; ok {
			continue
		}
		seen[c.Name] = struct{}{}
		out = append(out, c.Name)
	}
	for _, c := range liveRows {
		if _, ok := seen[c.Name]; ok {
			continue
		}
		seen[c.Name] = struct{}{}
		out = append(out, c.Name)
	}
	return out
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
