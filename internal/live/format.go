package live

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// FormatPodTable writes a kube-capacity-like text table (pod grain).
func FormatPodTable(w io.Writer, rows []PodRow) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAMESPACE\tPOD\tCPU_UTIL\tCPU_REQ\tCPU_RATIO\tMEM_UTIL\tMEM_REQ\tMEM_RATIO\tNODE")
	for _, r := range rows {
		cpuRatio := "n/a"
		if v, ok := r.CPURatioRequest(); ok {
			cpuRatio = fmt.Sprintf("%.2f", v)
		}
		memRatio := "n/a"
		if v, ok := r.MemRatioRequest(); ok {
			memRatio = fmt.Sprintf("%.2f", v)
		}
		cpuUtil := "n/a"
		if r.HasUtil {
			cpuUtil = FormatMillicores(r.CPUUtilMcores)
		}
		memUtil := "n/a"
		if r.HasUtil {
			memUtil = FormatBytes(r.MemUtilBytes)
		}
		node := r.Node
		if node == "" {
			node = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Namespace,
			r.Pod,
			cpuUtil,
			FormatMillicores(r.CPURequestMcores),
			cpuRatio,
			memUtil,
			FormatBytes(r.MemRequestBytes),
			memRatio,
			node,
		)
	}
	return tw.Flush()
}

// FormatMillicores formats millicores as e.g. 100m.
func FormatMillicores(m float64) string {
	if m == 0 {
		return "0"
	}
	if m == float64(int64(m)) {
		return fmt.Sprintf("%dm", int64(m))
	}
	return fmt.Sprintf("%.0fm", m)
}

// FormatBytes formats bytes as MiB.
func FormatBytes(b int64) string {
	if b == 0 {
		return "0"
	}
	const mi = 1024 * 1024
	if b%mi == 0 {
		return fmt.Sprintf("%dMi", b/mi)
	}
	return fmt.Sprintf("%.1fMi", float64(b)/float64(mi))
}

// ParseSortKey maps CLI/config sort strings to SortKey.
func ParseSortKey(s string) SortKey {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "name":
		return SortName
	case "cpu.util":
		return SortCPUUtil
	case "mem.util":
		return SortMemUtil
	case "mem.ratio":
		return SortMemRatio
	case "cpu.ratio", "":
		return SortCPURatio
	default:
		return SortCPURatio
	}
}
