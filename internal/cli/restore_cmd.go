package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/store"
)

func newRestoreCmd() *cobra.Command {
	var configPath string
	var merge bool
	var force bool

	cmd := &cobra.Command{
		Use:   "restore <backup-file>",
		Short: "Restore a backup into the store, or merge what it adds",
		Long: `Restore a backup written by "kpm backup" into this installation's store.

By default the store is replaced: the backup is validated, copied beside the
destination and renamed into place, then any stale -wal/-shm left by the previous
database is removed. Stop the sampler first -- a writer still holding the old file
keeps writing to an inode nothing points at any more.

With --merge the backup is opened read-only and only the rows this store does not
already hold are added, matched on the natural key of each table. Merge is safe to
repeat: running the same file twice adds nothing the second time.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			path, err := cfg.ExpandedStorePath()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if !merge {
				if err := store.RestoreReplace(args[0], path, force); err != nil {
					return exitcode.New(exitcode.Store, err)
				}
				_, _ = fmt.Fprintf(out, "restored %s → %s\n", args[0], path)
				return nil
			}

			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return exitcode.New(exitcode.Store, err)
			}
			st, err := store.Open(path)
			if err != nil {
				return exitcode.New(exitcode.Store, err)
			}
			defer func() { _ = st.Close() }()

			rep, err := st.MergeFrom(cmd.Context(), args[0])
			if err != nil {
				return exitcode.New(exitcode.Store, err)
			}
			_, _ = fmt.Fprintf(out, "merged %s (schema v%d) → %s\n", rep.Source, rep.SchemaVersion, st.Path())
			_, _ = fmt.Fprintf(out, "  samples         +%d\n", rep.Samples)
			_, _ = fmt.Fprintf(out, "  recommendations +%d\n", rep.Recommendations)
			_, _ = fmt.Fprintf(out, "  node_snapshots  +%d\n", rep.NodeSnapshots)
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().BoolVar(&merge, "merge", false, "add the rows the store is missing instead of replacing it")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "replace a non-empty store without refusing")
	return cmd
}
