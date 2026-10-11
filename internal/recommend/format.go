package recommend

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// FormatTable writes a human-readable advice table.
func FormatTable(w io.Writer, advice []Advice) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "GRAIN\tKEY\tSAMPLES\tCPU_P\tCPU_REQ\tCPU_SUG\tCPU_VER\tMEM_P\tMEM_REQ\tMEM_SUG\tMEM_VER")
	for _, a := range advice {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%sm\t%sm\t%sm\t%s\t%s\t%s\t%s\t%s\n",
			a.Grain,
			a.Key,
			a.Samples,
			formatM(a.CPUPercentile),
			formatM(a.CPURequest),
			formatM(a.CPUSuggested),
			a.CPUVerdictReq,
			formatBytes(int64(a.MemPercentile)),
			formatBytes(a.MemRequest),
			formatBytes(a.MemSuggested),
			a.MemVerdictReq,
		)
	}
	return tw.Flush()
}

// FormatTSV writes advice as tab-separated values with a header.
func FormatTSV(w io.Writer, advice []Advice) error {
	header := []string{
		"grain", "key", "namespace", "pod", "container", "owner_kind", "owner_name",
		"samples", "cpu_p_mcores", "cpu_req_mcores", "cpu_sug_mcores", "cpu_verdict_req",
		"mem_p_bytes", "mem_req_bytes", "mem_sug_bytes", "mem_verdict_req",
		"cpu_verdict_limit", "mem_verdict_limit",
	}
	if _, err := fmt.Fprintln(w, strings.Join(header, "\t")); err != nil {
		return err
	}
	for _, a := range advice {
		line := []string{
			string(a.Grain),
			a.Key,
			a.Namespace,
			a.Pod,
			a.Container,
			a.OwnerKind,
			a.OwnerName,
			fmt.Sprintf("%d", a.Samples),
			fmt.Sprintf("%.0f", a.CPUPercentile),
			fmt.Sprintf("%.0f", a.CPURequest),
			fmt.Sprintf("%.0f", a.CPUSuggested),
			string(a.CPUVerdictReq),
			fmt.Sprintf("%d", int64(a.MemPercentile)),
			fmt.Sprintf("%d", a.MemRequest),
			fmt.Sprintf("%d", a.MemSuggested),
			string(a.MemVerdictReq),
			string(a.CPUVerdictLimit),
			string(a.MemVerdictLimit),
		}
		if _, err := fmt.Fprintln(w, strings.Join(line, "\t")); err != nil {
			return err
		}
	}
	return nil
}

func formatM(m float64) string {
	if m == 0 {
		return "0"
	}
	return fmt.Sprintf("%.0f", m)
}

func formatBytes(b int64) string {
	if b == 0 {
		return "0"
	}
	const mi = 1024 * 1024
	if b%mi == 0 {
		return fmt.Sprintf("%dMi", b/mi)
	}
	return fmt.Sprintf("%.1fMi", float64(b)/float64(mi))
}
