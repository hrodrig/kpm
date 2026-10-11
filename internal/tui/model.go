package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/store"
)

// SnapshotFn captures a live snapshot.
type SnapshotFn func(ctx context.Context) (live.Snapshot, error)

// RecsFn loads latest recommendation snapshots (nil/empty OK).
type RecsFn func(ctx context.Context) ([]store.Recommendation, error)

// Config holds TUI runtime knobs.
type Config struct {
	Refresh   time.Duration
	Sort      live.SortKey
	Baseline  Baseline
	ShowDelta bool
	Version   string // e.g. v0.2.0 from CLI ldflags
}

type tickMsg time.Time

type snapMsg struct {
	snap live.Snapshot
	idx  RecIndex
	err  error
}

// Model is the bubbletea TUI state.
type Model struct {
	cfg      Config
	snapshot SnapshotFn
	recs     RecsFn
	ctx      context.Context

	width  int
	height int
	offset int
	cursor int

	metric Metric
	rows   []DisplayRow
	errMsg string
	ready  bool
}

// NewModel builds the interactive model.
func NewModel(ctx context.Context, cfg Config, snap SnapshotFn, recs RecsFn) Model {
	if cfg.Refresh < time.Second {
		cfg.Refresh = 5 * time.Second
	}
	if cfg.Sort == "" {
		cfg.Sort = live.SortCPURatio
	}
	return Model{
		cfg:      cfg,
		snapshot: snap,
		recs:     recs,
		ctx:      ctx,
		metric:   MetricCPU,
	}
}

// Init starts the refresh ticker and first capture.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd(m.cfg.Refresh))
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		snap, err := m.snapshot(m.ctx)
		if err != nil {
			return snapMsg{err: err}
		}
		idx := IndexRecommendations(nil)
		if m.recs != nil {
			recs, rerr := m.recs(m.ctx)
			if rerr != nil {
				return snapMsg{snap: snap, idx: idx, err: fmt.Errorf("recommendations: %w", rerr)}
			}
			idx = IndexRecommendations(recs)
		}
		return snapMsg{snap: snap, idx: idx}
	}
}

// Update handles keys, ticks, and snapshot results.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tickCmd(m.cfg.Refresh))
	case snapMsg:
		return m.applySnap(msg), nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) applySnap(msg snapMsg) Model {
	if msg.err != nil && len(msg.snap.Containers) == 0 {
		m.errMsg = msg.err.Error()
		return m
	}
	if msg.err != nil {
		m.errMsg = msg.err.Error()
	} else {
		m.errMsg = ""
	}
	m.rows = PrepareRows(msg.snap, msg.idx, m.cfg.Sort, m.metric, m.cfg.Baseline)
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.clampOffset()
	return m
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
			m.clampOffset()
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.clampOffset()
		}
	case "s":
		m.cfg.Sort = CycleSort(m.cfg.Sort)
		return m, m.refreshCmd()
	case "m":
		if m.metric == MetricCPU {
			m.metric = MetricMem
		} else {
			m.metric = MetricCPU
		}
		return m, m.refreshCmd()
	case "b":
		if m.cfg.Baseline == BaselineRequest {
			m.cfg.Baseline = BaselineLimit
		} else {
			m.cfg.Baseline = BaselineRequest
		}
		return m, m.refreshCmd()
	case "d":
		m.cfg.ShowDelta = !m.cfg.ShowDelta
	}
	return m, nil
}

func (m *Model) clampOffset() {
	body := m.bodyHeight()
	if body < 1 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+body {
		m.offset = m.cursor - body + 1
	}
}

func (m Model) bodyHeight() int {
	h := m.height - 3 // header + blank + footer
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the TUI.
func (m Model) View() string {
	if !m.ready && m.width == 0 {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(HeaderLine(m.cfg.Version, len(m.rows), m.metric, m.cfg.Baseline, m.cfg.Sort, m.cfg.ShowDelta, m.errMsg))
	b.WriteByte('\n')
	idWidth := 40
	barWidth := 20
	if m.width > 60 {
		idWidth = m.width - 40
		if m.cfg.ShowDelta {
			idWidth = m.width - 55
		}
		if idWidth < 16 {
			idWidth = 16
		}
		barWidth = 16
		if m.width > 100 {
			barWidth = 24
		}
	}
	body := m.bodyHeight()
	end := m.offset + body
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		r := m.rows[i]
		cur := " "
		if i == m.cursor {
			cur = ">"
		}
		line := fmt.Sprintf("%s %-*s %s %4s",
			cur,
			idWidth, FormatIdentity(r.Namespace, r.Pod, idWidth),
			RenderBar(r.Ratio, r.HasRatio, barWidth),
			FormatPct(r.Ratio, r.HasRatio),
		)
		if m.cfg.ShowDelta {
			line += fmt.Sprintf("  Δcpu %6s  Δmem %7s", FormatCPUDelta(r.Delta), FormatMemDelta(r.Delta))
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if len(m.rows) == 0 {
		b.WriteString("(no pods)\n")
	}
	return b.String()
}
