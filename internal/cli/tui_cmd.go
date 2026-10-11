package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/store"
	"github.com/hrodrig/kpm/internal/tui"
)

func newTUICmd() *cobra.Command {
	var configPath string
	var kubeconfig string

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Interactive live heatmap of pod util ratios",
		Long: `Full-screen TUI (bubbletea) over live Metrics API data.
Shows util/request (or util/limit) bars; optional Δ vs last recommend snapshots
from the SQLite store (read-only). Does not write the store.
Also available as: kpm capacity --tui`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd, configPath, kubeconfig)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: discovery / in-cluster)")
	return cmd
}

func runTUI(cmd *cobra.Command, configPath, kubeconfig string) error {
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

	// Read-only join to existing store for Δ column; never create a new DB here.
	var st *store.Store
	if path, err := cfg.ExpandedStorePath(); err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if opened, oerr := store.Open(path); oerr == nil {
				st = opened
				defer st.Close()
			}
		}
	}

	if err := tui.Run(cmd.Context(), cfg, cs, mcs, st, Version); err != nil {
		return exitcode.New(exitcode.Cluster, err)
	}
	return nil
}
