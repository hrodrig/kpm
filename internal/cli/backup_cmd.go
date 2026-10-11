package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func newBackupCmd() *cobra.Command {
	var configPath string
	var output string

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Copy the SQLite store to a backup file (VACUUM INTO)",
		Args:  cobra.NoArgs,
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

			dest := output
			if dest == "" {
				dest = filepath.Join(
					filepath.Dir(path),
					fmt.Sprintf("metrics-backup-%s.db", time.Now().UTC().Format("20060102T150405Z")),
				)
			}
			if err := st.Backup(cmd.Context(), dest); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "backup written → %s\n", dest)
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVarP(&output, "output", "o", "", "backup destination path (default: alongside store)")
	return cmd
}
