package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/client-go/kubernetes"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/store"
)

// Run starts the interactive TUI (blocks until quit).
// version is the CLI build version (e.g. v0.2.0); shown in the header.
func Run(ctx context.Context, cfg *config.Config, cs kubernetes.Interface, mcs metricsclientset.Interface, st *store.Store, version string) error {
	opt := live.Options{
		Scope:          cfg.Scope,
		PodAllowlist:   cfg.TUI.Pods,
		ExtraPodLabels: cfg.TUI.PodLabels,
	}
	snapFn := func(c context.Context) (live.Snapshot, error) {
		return live.CaptureClients(c, cs, mcs, opt)
	}
	var recsFn RecsFn
	if st != nil {
		recsFn = func(c context.Context) ([]store.Recommendation, error) {
			return st.LatestRecommendations(c, "")
		}
	}
	sec := cfg.TUI.RefreshSeconds
	if sec < 1 {
		sec = 5
	}
	tuiCfg := Config{
		Refresh:   time.Duration(sec) * time.Second,
		Sort:      live.ParseSortKey(cfg.TUI.DefaultSort),
		Baseline:  ParseBaseline(cfg.TUI.Baseline),
		ShowDelta: true,
		Version:   version,
	}
	m := NewModel(ctx, tuiCfg, snapFn, recsFn)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
