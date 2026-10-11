package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/mcp"
	"github.com/hrodrig/kpm/internal/store"
)

func newMCPCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run read-only MCP server over stdio (store tools)",
		Long: `Serve kpm store tools for MCP clients over stdio (Cursor, Claude, Codex, Hermes, ...).

Tools: get_status, list_samples, summarize_window, list_recommendations.
Does not require the sample daemon. Opens the store using store.path from
--config. Protocol logs must go to stderr only.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
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

			deps := &mcp.Deps{
				Store:   st,
				Config:  cfg,
				Version: Version,
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "kpm mcp: stdio tools on store %s\n", st.Path())
			return mcp.RunStdio(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	return cmd
}
