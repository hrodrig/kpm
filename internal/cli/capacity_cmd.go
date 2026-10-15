package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/identity"
	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/store"
)

func newCapacityCmd() *cobra.Command {
	var configPath string
	var kubeconfig string
	var sortFlag string
	var asTUI bool
	var recordNodes bool

	cmd := &cobra.Command{
		Use:   "capacity",
		Short: "Live pod CPU/memory capacity table (optional node snapshots)",
		Long: `One-shot live table from the Metrics API (pod-aggregated: sum util / sum request).
Does not read the SQLite store unless --record-nodes is set (writes node_snapshots).
Exit code 2 on cluster/Metrics/RBAC failure. With --tui, enters the interactive heatmap
(same as kpm tui).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if asTUI {
				return runTUI(cmd, configPath, kubeconfig)
			}
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

			if err := writeCapacityTable(cmd.OutOrStdout(), cmd.ErrOrStderr(), snap, sortFlag, cfg.TUI.DefaultSort); err != nil {
				return err
			}
			if !recordNodes {
				return nil
			}
			return recordNodeSnapshots(cmd, cfg, cs, snap)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: discovery / in-cluster)")
	cmd.Flags().StringVar(&sortFlag, "sort", "", "sort key: name, cpu.ratio, cpu.util, mem.ratio, mem.util (default: tui.default_sort or cpu.ratio)")
	cmd.Flags().BoolVar(&asTUI, "tui", false, "open interactive heatmap (alias for kpm tui)")
	cmd.Flags().BoolVar(&recordNodes, "record-nodes", false, "persist node allocatable/request/util into node_snapshots")
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

func recordNodeSnapshots(cmd *cobra.Command, cfg *config.Config, cs kubernetes.Interface, snap live.Snapshot) error {
	nodes, err := live.CaptureNodeCapacityClients(cmd.Context(), cs, snap.Containers)
	if err != nil {
		return exitcode.New(exitcode.Cluster, err)
	}
	path, err := cfg.ExpandedStorePath()
	if err != nil {
		return err
	}
	st, err := store.Open(path)
	if err != nil {
		return exitcode.New(exitcode.Store, err)
	}
	defer st.Close()

	now := time.Now().UTC()
	if !snap.CapturedAt.IsZero() {
		now = snap.CapturedAt.UTC()
	}
	clientID := identity.Resolve(cfg.Client.ID)
	clusterName := cfg.Cluster.Name
	if clusterName == "" {
		clusterName = "-"
	}
	rows := make([]store.NodeSnapshot, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, store.NodeSnapshot{
			RecordedAt:     now,
			ClientID:       clientID,
			ClusterName:    clusterName,
			Node:           n.Name,
			CPUAllocMcores: n.CPUAllocMcores,
			MemAllocBytes:  n.MemAllocBytes,
			CPUCapMcores:   n.CPUCapMcores,
			MemCapBytes:    n.MemCapBytes,
			CPUReqMcores:   n.CPUReqMcores,
			MemReqBytes:    n.MemReqBytes,
			CPUUtilMcores:  n.CPUUtilMcores,
			MemUtilBytes:   n.MemUtilBytes,
			Pods:           n.Pods,
		})
	}
	if err := st.InsertNodeSnapshots(cmd.Context(), rows); err != nil {
		return exitcode.New(exitcode.Store, err)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "capacity: recorded %d node snapshot rows → %s\n", len(rows), st.Path())
	return nil
}
