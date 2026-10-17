package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// FormatTable writes one greppable line per check plus a summary line.
func FormatTable(w io.Writer, r Report) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL"); err != nil {
		return err
	}
	for _, c := range r.Checks {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, c.Status, c.Detail); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "doctor: %d ok, %d warn, %d fail, %d skipped (status %s)\n",
		r.Counts.OK, r.Counts.Warn, r.Counts.Fail, r.Counts.Skip, r.Status)
	return err
}

// FormatJSON writes the machine-readable report. Field names follow the server
// API convention: lower snake case, explicit aggregate counts alongside the rows.
func FormatJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
