// Package cli implements the kpm Cobra command tree.
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/hrodrig/kpm/configs"
	"github.com/spf13/cobra"
)

// errPrintSampleDone stops command execution after sample YAML was written to stdout.
var errPrintSampleDone = errors.New("sample config printed")

var (
	printSampleConfig bool
)

// Execute runs the root command and returns any error (mapped to an exit code
// by cmd/kpm/main.go). Prefer ExecuteContext from main so SIGINT/SIGTERM
// cancel a future sampler daemon cleanly.
func Execute() error {
	return ExecuteContext(context.Background())
}

// ExecuteContext runs the root command with ctx (typically signal.NotifyContext).
func ExecuteContext(ctx context.Context) error {
	err := NewRootCmd().ExecuteContext(ctx)
	if errors.Is(err, errPrintSampleDone) {
		return nil
	}
	return err
}

// NewRootCmd builds the kpm command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "kpm",
		Short:         "Kubernetes Pods Metrics — sample CPU/mem into SQLite and recommend rightsizing",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if !printSampleConfig {
				return nil
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), configs.SampleYAML())
			if err != nil {
				return err
			}
			return errPrintSampleDone
		},
	}

	root.PersistentFlags().BoolVar(&printSampleConfig, "print-sample-config", false, "print sample kpm.yml to stdout and exit")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newSampleCmd())
	root.AddCommand(newBackupCmd())
	root.AddCommand(newExportCmd())
	return root
}
