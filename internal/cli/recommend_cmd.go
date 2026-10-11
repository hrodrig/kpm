package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/recommend"
	"github.com/hrodrig/kpm/internal/store"
)

func newRecommendCmd() *cobra.Command {
	var configPath string
	var format string
	var namespace string
	var workload string

	cmd := &cobra.Command{
		Use:   "recommend",
		Short: "Offline rightsizing advice from SQLite history",
		Long: `Compute CPU/memory request suggestions from store samples (no cluster access).
Dual grain: workload+container and pod+container. Exit code 3 when no key meets min_samples.`,
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

			now := time.Now().UTC()
			since := now.AddDate(0, 0, -cfg.Recommend.WindowDays)
			recs, err := st.ListRecords(cmd.Context(), store.ExportFilter{
				Since:     since,
				Until:     now.Add(time.Second), // include "now"
				Namespace: namespace,
				Workload:  workload,
			})
			if err != nil {
				return err
			}

			res := recommend.Run(recs, cfg.Recommend, now)
			return writeRecommend(cmd.OutOrStdout(), cmd.ErrOrStderr(), res, format)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or tsv")
	cmd.Flags().StringVar(&namespace, "namespace", "", "filter samples by namespace")
	cmd.Flags().StringVar(&workload, "workload", "", "filter samples by owner_name")
	return cmd
}

func writeRecommend(out, errOut io.Writer, res recommend.Result, format string) error {
	if len(res.Advice) == 0 {
		_, _ = fmt.Fprintf(errOut, "recommend: no keys with ≥ %d samples (skipped %d keys, %d rows in window)\n",
			res.MinSamples, res.SkippedKeys, res.RecordsInWin)
		return exitcode.New(exitcode.Store, recommend.ErrInsufficient)
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "tsv":
		if err := recommend.FormatTSV(out, res.Advice); err != nil {
			return err
		}
	case "table", "":
		if err := recommend.FormatTable(out, res.Advice); err != nil {
			return err
		}
	default:
		return fmt.Errorf("recommend --format must be table or tsv (got %q)", format)
	}
	_, _ = fmt.Fprintf(errOut, "recommend: %d advice rows (skipped %d keys below min_samples)\n",
		len(res.Advice), res.SkippedKeys)
	return nil
}
