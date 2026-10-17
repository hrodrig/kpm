package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/doctor"
	"github.com/hrodrig/kpm/internal/exitcode"
)

func newDoctorCmd() *cobra.Command {
	var configPath string
	var kubeconfig string
	var format string
	var live bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Read-only diagnosis of config, store, history coverage and cluster",
		Long: `Check each layer that has to work for trustworthy recommendations, and report
every one of them instead of stopping at the first problem.

Read-only by design: the store is opened without creating or migrating it,
nothing is sampled, and no cluster connection is made unless --live is passed
(kpm serve and kpm mcp are documented as usable offline against a copied SQLite
file, so a missing cluster is not a broken setup).

Exit codes: 0 when nothing verifiably broke — warnings are "look at this", not
failures — 1 for a general failure, and 2 when the only failures come from the
cluster probe.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := doctor.Run(cmd.Context(), doctor.Options{
				ConfigPath: configPath,
				Kubeconfig: kubeconfig,
				Live:       live,
			})

			out := cmd.OutOrStdout()
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "json":
				if err := doctor.FormatJSON(out, report); err != nil {
					return err
				}
			case "table", "":
				if err := doctor.FormatTable(out, report); err != nil {
					return err
				}
			default:
				return fmt.Errorf("doctor --format must be table or json (got %q)", format)
			}

			if report.Broken() {
				return exitcode.New(report.ExitCode(),
					fmt.Errorf("doctor: %d check(s) failed", report.Counts.Fail))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (used only with --live)")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or json")
	cmd.Flags().BoolVar(&live, "live", false, "also probe the cluster / Metrics API")
	return cmd
}
