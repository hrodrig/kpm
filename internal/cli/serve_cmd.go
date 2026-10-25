package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/server"
	"github.com/hrodrig/kpm/internal/store"
)

func newServeCmd() *cobra.Command {
	var configPath string
	var kubeconfig string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve ops + MCP/JSON HTTP against an existing store (no sample loop)",
		Long: `Analyze-local HTTP facade: open store.path, bind shared listen, serve
/livez /healthz /readyz /metrics plus MCP and/or JSON /api/v1 until SIGINT/SIGTERM.

Requires mcp.enabled and/or server.enabled. Does not scrape Metrics or write
samples — use for laptop analysis of a copied SQLite DB
(sample.interval_seconds may be 0). Kubeconfig is optional; /readyz only
checks the store. GET /api/v1/live returns 503 JSON when kube is unavailable.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if !cfg.MCP.Enabled && !cfg.Server.Enabled {
				return fmt.Errorf("kpm serve requires mcp.enabled and/or server.enabled")
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

			liveFn, liveErr := optionalLive(kubeconfig, cfg)
			if liveErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"kpm serve: kube unavailable (%v); /api/v1/live will 503\n", liveErr)
			}

			srv, err := newOpsHTTP(cfg, st, liveFn)
			if err != nil {
				return err
			}
			if srv == nil {
				return fmt.Errorf("kpm serve: HTTP facade not built")
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"kpm serve: store=%s listen=%s", st.Path(), srv.Addr())
			if cfg.MCP.Enabled {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), " mcp=%s+%s", mcp.PathSSE, mcp.PathMCP)
			}
			if cfg.Server.Enabled {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), " api=%s ui=/", server.APIPrefix)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			return srv.ListenAndServe(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "optional kubeconfig for /api/v1/live")
	return cmd
}

// optionalLive returns a live capture func when kube + metrics clients work.
func optionalLive(kubeconfig string, cfg *config.Config) (func(context.Context) (server.LiveOut, error), error) {
	restCfg, err := cluster.LoadRESTConfig(kubeconfig, cfg.Kube.Context)
	if err != nil {
		return nil, err
	}
	cs, err := cluster.NewClientset(restCfg)
	if err != nil {
		return nil, err
	}
	mcs, err := cluster.NewMetricsClientset(restCfg)
	if err != nil {
		return nil, err
	}
	return liveFnFromClients(cs, mcs, cfg.Scope), nil
}
