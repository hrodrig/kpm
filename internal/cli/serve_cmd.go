package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/store"
)

func newServeCmd() *cobra.Command {
	var configPath string
	var kubeconfig string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve ops + MCP HTTP against an existing store (no sample loop)",
		Long: `Analyze-local HTTP facade: open store.path, bind mcp.listen, serve
/healthz /readyz /metrics and MCP (/sse, /mcp) until SIGINT/SIGTERM.

Requires mcp.enabled. Does not scrape Metrics or write samples — use for
laptop analysis of a copied SQLite DB (sample.interval_seconds may be 0).
Kubeconfig is optional; /readyz only checks the store.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if !cfg.MCP.Enabled {
				return fmt.Errorf("kpm serve requires mcp.enabled: true (charts/server slice later)")
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

			cs, csErr := optionalClientset(kubeconfig, cfg.Kube.Context)
			if csErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"kpm serve: kube unavailable (%v); continuing store-only\n", csErr)
			}

			srv, err := newOpsHTTP(cfg, st, cs)
			if err != nil {
				return err
			}
			if srv == nil {
				return fmt.Errorf("kpm serve: ops HTTP not built (mcp.enabled?)")
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"kpm serve: store=%s listen=%s (/healthz /readyz /metrics; MCP %s + %s)\n",
				st.Path(), srv.Addr(), mcp.PathSSE, mcp.PathMCP)
			return srv.ListenAndServe(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "optional kubeconfig (live enrichment later; not required for ready)")
	return cmd
}

// optionalClientset returns a clientset when kubeconfig/discovery works.
// On failure returns (nil, err) so callers can continue store-only.
func optionalClientset(kubeconfig, context string) (kubernetes.Interface, error) {
	restCfg, err := cluster.LoadRESTConfig(kubeconfig, context)
	if err != nil {
		return nil, err
	}
	return cluster.NewClientset(restCfg)
}
