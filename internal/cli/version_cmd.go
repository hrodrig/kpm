package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// newVersionCmd builds `kpm version`.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build metadata",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "kpm %s\n", Version)
			_, _ = fmt.Fprintf(out, "  commit: %s\n", Commit)
			_, _ = fmt.Fprintf(out, "  built:  %s\n", BuildDate)
			_, _ = fmt.Fprintf(out, "  branch: %s\n", Branch)
			_, _ = fmt.Fprintf(out, "  go:     %s\n", runtime.Version())
		},
	}
}
