package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/identity"
	"github.com/hrodrig/kpm/internal/sample"
	"github.com/hrodrig/kpm/internal/store"
)

func newSampleCmd() *cobra.Command {
	var configPath string
	var kubeconfig string

	cmd := &cobra.Command{
		Use:   "sample",
		Short: "Sample pod CPU/memory into the local SQLite store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if cfg.Sample.IntervalSeconds > 0 {
				return fmt.Errorf("daemon sample loop (interval_seconds > 0) lands in roadmap #4; use interval_seconds: 0 for one-shot")
			}

			path, err := cfg.ExpandedStorePath()
			if err != nil {
				return err
			}
			st, err := store.Open(path)
			if err != nil {
				return err
			}
			defer st.Close()

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

			res, err := sample.RunOnce(cmd.Context(), cfg, st, cs, mcs)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}
			clientID := identity.Resolve(cfg.Client.ID)
			clusterName := cfg.Cluster.Name
			if clusterName == "" {
				clusterName = "-"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"sampled %d containers (%d pods skipped, no metrics) client.id=%s cluster=%s → %s\n",
				len(res.Records), res.SkippedPods, clientID, clusterName, st.Path())
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: discovery / in-cluster)")
	return cmd
}
