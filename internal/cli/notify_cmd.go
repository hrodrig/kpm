package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/identity"
	"github.com/hrodrig/kpm/internal/notify"
)

func newNotifyTestCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "notify-test",
		Short: "Send a synthetic firing notification to all configured sinks",
		Long: `Loads notifications.sinks from config and fans out one test alert.
Does not sample the cluster or wait for hold_for_seconds.
Fail-closed if notifications.enabled and any sink env is missing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if !cfg.Notifications.Enabled {
				return fmt.Errorf("notifications.enabled is false; enable to use notify-test")
			}
			senders, err := notify.BuildSenders(cfg.Notifications.Sinks)
			if err != nil {
				return err
			}
			clientID := identity.Resolve(cfg.Client.ID)
			cluster := cfg.Cluster.Name
			if cluster == "" {
				cluster = "-"
			}
			msg := notify.BuildMessage(
				notify.EventFiring,
				clientID,
				cluster,
				"notify-test",
				"cpu.util_vs_request",
				0.95,
				0.80,
				"demo",
				"notify-test",
				"main",
				time.Now().UTC(),
			)
			if err := notify.FanOut(cmd.Context(), senders, msg); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"notify-test: sent firing event to %d sink(s) client.id=%s cluster=%s\n",
				len(senders), clientID, cluster)
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to kpm YAML config")
	return cmd
}
