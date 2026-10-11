package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/identity"
	"github.com/hrodrig/kpm/internal/recommend"
	"github.com/hrodrig/kpm/internal/store"
)

func newRecommendCmd() *cobra.Command {
	var configPath string
	var format string
	var namespace string
	var workload string
	var record bool
	var score bool

	cmd := &cobra.Command{
		Use:   "recommend",
		Short: "Offline rightsizing advice from SQLite history",
		Long: `Compute CPU/memory request suggestions from store samples (no cluster access).
Dual grain: workload+container and pod+container. Exit code 3 when no key meets min_samples.

--record persists advice rows into the recommendations table (schema v2).
--score evaluates latest snapshots against later samples (calibration mode A).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if record && score {
				return fmt.Errorf("recommend: --record and --score are mutually exclusive")
			}
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

			if score {
				return runRecommendScore(cmd, st, cfg, namespace, workload)
			}
			return runRecommendAdvice(cmd, st, cfg, format, namespace, workload, record)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or tsv")
	cmd.Flags().StringVar(&namespace, "namespace", "", "filter samples by namespace")
	cmd.Flags().StringVar(&workload, "workload", "", "filter samples by owner_name")
	cmd.Flags().BoolVar(&record, "record", false, "persist advice into recommendations table")
	cmd.Flags().BoolVar(&score, "score", false, "score latest recommendations vs later samples")
	return cmd
}

func runRecommendAdvice(cmd *cobra.Command, st *store.Store, cfg *config.Config, format, namespace, workload string, record bool) error {
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -cfg.Recommend.WindowDays)
	recs, err := st.ListRecords(cmd.Context(), store.ExportFilter{
		Since:     since,
		Until:     now.Add(time.Second),
		Namespace: namespace,
		Workload:  workload,
	})
	if err != nil {
		return err
	}

	res := recommend.Run(recs, cfg.Recommend, now)
	if err := writeRecommend(cmd.OutOrStdout(), cmd.ErrOrStderr(), res, format); err != nil {
		return err
	}
	if !record {
		return nil
	}
	clientID := identity.Resolve(cfg.Client.ID)
	clusterName := cfg.Cluster.Name
	if clusterName == "" {
		clusterName = "-"
	}
	rows := recommend.SnapshotsFromResult(res, clientID, clusterName, now)
	if err := st.InsertRecommendations(cmd.Context(), rows); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "recommend: recorded %d recommendation rows\n", len(rows))
	return nil
}

func runRecommendScore(cmd *cobra.Command, st *store.Store, cfg *config.Config, namespace, workload string) error {
	snaps, err := st.LatestRecommendations(cmd.Context(), "")
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "recommend --score: no recommendation snapshots in store")
		return exitcode.New(exitcode.Store, fmt.Errorf("no recommendations to score"))
	}
	samples, err := st.ListRecords(cmd.Context(), store.ExportFilter{
		Namespace: namespace,
		Workload:  workload,
	})
	if err != nil {
		return err
	}
	res := recommend.ScoreCalibration(snaps, samples, recommend.ScoreOptions{
		MinSamples: cfg.Recommend.MinSamples,
		Percentile: cfg.Recommend.Percentile,
	})
	if err := recommend.FormatScoreTable(cmd.OutOrStdout(), res); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "recommend --score: %d keys (%d skipped below min_samples)\n",
		len(res.Rows), res.Skipped)
	return nil
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
