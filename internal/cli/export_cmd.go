package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

type exportFlags struct {
	configPath string
	format     string
	output     string
	sinceStr   string
	untilStr   string
	namespace  string
	pod        string
	workload   string
}

func newExportCmd() *cobra.Command {
	var f exportFlags
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export samples from SQLite as csv or tsv",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExport(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	cmd.Flags().StringVar(&f.format, "format", "csv", "csv or tsv")
	cmd.Flags().StringVarP(&f.output, "output", "o", "-", "output file (default stdout)")
	cmd.Flags().StringVar(&f.sinceStr, "since", "", "RFC3339 inclusive lower bound")
	cmd.Flags().StringVar(&f.untilStr, "until", "", "RFC3339 exclusive upper bound")
	cmd.Flags().StringVar(&f.namespace, "namespace", "", "filter namespace")
	cmd.Flags().StringVar(&f.pod, "pod", "", "filter pod name")
	cmd.Flags().StringVar(&f.workload, "workload", "", "filter owner_name (workload)")
	return cmd
}

func runExport(cmd *cobra.Command, f exportFlags) error {
	cfg, err := config.Load(f.configPath)
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

	filt, err := buildExportFilter(f)
	if err != nil {
		return err
	}
	fmtNorm := store.ExportFormat(strings.ToLower(strings.TrimSpace(f.format)))

	w, closer, err := exportWriter(cmd, f.output)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}

	n, err := st.Export(cmd.Context(), w, fmtNorm, filt)
	if err != nil {
		return err
	}
	if f.output != "" && f.output != "-" {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "exported %d rows → %s\n", n, f.output)
	}
	return nil
}

func buildExportFilter(f exportFlags) (store.ExportFilter, error) {
	filt := store.ExportFilter{
		Namespace: f.namespace,
		Pod:       f.pod,
		Workload:  f.workload,
	}
	if f.sinceStr != "" {
		t, err := time.Parse(time.RFC3339, f.sinceStr)
		if err != nil {
			return filt, fmt.Errorf("--since must be RFC3339: %w", err)
		}
		filt.Since = t
	}
	if f.untilStr != "" {
		t, err := time.Parse(time.RFC3339, f.untilStr)
		if err != nil {
			return filt, fmt.Errorf("--until must be RFC3339: %w", err)
		}
		filt.Until = t
	}
	return filt, nil
}

func exportWriter(cmd *cobra.Command, output string) (io.Writer, io.Closer, error) {
	if output == "" || output == "-" {
		return cmd.OutOrStdout(), nil, nil
	}
	f, err := os.Create(output)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}
