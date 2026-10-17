package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/fit"
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
	var listNodes bool

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
			if !listNodes && !recordNodes {
				return nil
			}
			nodes, err := live.CaptureNodeCapacityClients(cmd.Context(), cs, snap.Containers)
			if err != nil {
				return exitcode.New(exitcode.Cluster, err)
			}
			if listNodes {
				if err := writeNodeTable(cmd.OutOrStdout(), nodes); err != nil {
					return err
				}
			}
			if !recordNodes {
				return nil
			}
			return recordNodeSnapshots(cmd, cfg, nodes, snap.CapturedAt)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: discovery / in-cluster)")
	cmd.Flags().StringVar(&sortFlag, "sort", "", "sort key: name, cpu.ratio, cpu.util, mem.ratio, mem.util (default: tui.default_sort or cpu.ratio)")
	cmd.Flags().BoolVar(&asTUI, "tui", false, "open interactive heatmap (alias for kpm tui)")
	cmd.Flags().BoolVar(&recordNodes, "record-nodes", false, "persist node allocatable/request/util into node_snapshots")
	cmd.Flags().BoolVar(&listNodes, "nodes", false, "also print the per-node table with its pool (taints, cordon)")
	cmd.AddCommand(newCapacityFitCmd())
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

func recordNodeSnapshots(cmd *cobra.Command, cfg *config.Config, nodes []live.NodeCapacity, capturedAt time.Time) error {
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
	if !capturedAt.IsZero() {
		now = capturedAt.UTC()
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
			Taints:         n.Taints,
			Unschedulable:  n.Unschedulable,
		})
	}
	if err := st.InsertNodeSnapshots(cmd.Context(), rows); err != nil {
		return exitcode.New(exitcode.Store, err)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "capacity: recorded %d node snapshot rows → %s\n", len(rows), st.Path())
	return nil
}

// writeNodeTable prints per-node capacity with the pool each node belongs to,
// then the same numbers grouped by pool.
//
// No total is filtered by taint. A dedicated pool's capacity is real and appears
// here in full; the pool column says who may consume it, which is a different
// question from whether it exists.
func writeNodeTable(out io.Writer, nodes []live.NodeCapacity) error {
	_, _ = fmt.Fprintf(out, "\n%-30s %-38s %9s %9s %9s %6s\n",
		"NODE", "POOL", "ALLOC_CPU", "REQ_CPU", "UTIL_CPU", "PODS")
	for _, n := range nodes {
		cordoned := ""
		if n.Unschedulable {
			cordoned = " (cordoned)"
		}
		_, _ = fmt.Fprintf(out, "%-30s %-38s %9s %9s %9s %6d%s\n",
			n.Name, live.PoolKey(n),
			fit.FormatMilliCPU(n.CPUAllocMcores), fit.FormatMilliCPU(n.CPUReqMcores),
			fit.FormatMilliCPU(n.CPUUtilMcores), n.Pods, cordoned)
	}

	pools := live.GroupNodesByPool(nodes)
	if len(pools) == 0 {
		return nil
	}
	_, _ = fmt.Fprintf(out, "\n%-38s %5s %8s %9s %9s %9s %10s %10s\n",
		"POOL", "NODES", "CORDONED", "ALLOC_CPU", "REQ_CPU", "UTIL_CPU", "ALLOC_MEM", "REQ_MEM")
	for _, p := range pools {
		_, _ = fmt.Fprintf(out, "%-38s %5d %8d %9s %9s %9s %10s %10s\n",
			p.Pool, p.Nodes, p.Cordoned,
			fit.FormatMilliCPU(p.CPUAllocMcores), fit.FormatMilliCPU(p.CPUReqMcores),
			fit.FormatMilliCPU(p.CPUUtilMcores),
			fit.FormatBytes(p.MemAllocBytes), fit.FormatBytes(p.MemReqBytes))
	}
	return nil
}
