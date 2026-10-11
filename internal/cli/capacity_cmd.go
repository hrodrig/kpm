package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/live"
)

func newCapacityCmd() *cobra.Command {
	var configPath string
	var kubeconfig string
	var sortFlag string

	cmd := &cobra.Command{
		Use:   "capacity",
		Short: "Live pod CPU/memory capacity table (no SQLite)",
		Long: `One-shot live table from the Metrics API (pod-aggregated: sum util / sum request).
Does not read or write the SQLite store. Exit code 2 on cluster/Metrics/RBAC failure.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			restCfg, err := cluster.LoadRESTConfig(kubeconfig, cfg.Kube.Context)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}
			cs, err := cluster.NewClientset(restCfg)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}
			mcs, err := cluster.NewMetricsClientset(restCfg)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}

			snap, err := live.CaptureClients(cmd.Context(), cs, mcs, live.Options{
				Scope:          cfg.Scope,
				ExtraPodLabels: cfg.TUI.PodLabels,
			})
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}

			return writeCapacityTable(cmd.OutOrStdout(), cmd.ErrOrStderr(), snap, sortFlag, cfg.TUI.DefaultSort)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: discovery / in-cluster)")
	cmd.Flags().StringVar(&sortFlag, "sort", "", "sort key: name, cpu.ratio, cpu.util, mem.ratio, mem.util (default: tui.default_sort or cpu.ratio)")
	return cmd
}

// writeCapacityTable aggregates, sorts, and prints a capacity snapshot.
func writeCapacityTable(out, errOut io.Writer, snap live.Snapshot, sortFlag, defaultSort string) error {
	rows := live.AggregatePods(snap.Containers)
	key := live.ParseSortKey(sortFlag)
	if sortFlag == "" {
		key = live.ParseSortKey(defaultSort)
	}
	live.SortPods(rows, key)
	if err := live.FormatPodTable(out, rows); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(errOut, "capacity: %d pods\n", len(rows))
	return nil
}
