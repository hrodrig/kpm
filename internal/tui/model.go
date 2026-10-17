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

// OwnerFn loads Deploy/STS/DS template resources (read-only). Nil disables Enter detail.
type OwnerFn func(ctx context.Context, namespace, ownerKind, ownerName string) (live.OwnerTemplate, error)

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

type detailMsg struct {
	body string
	err  error
}

// Model is the bubbletea TUI state.
type Model struct {
	cfg      Config
	snapshot SnapshotFn
	recs     RecsFn
	owner    OwnerFn
	ctx      context.Context

	width  int
	height int
	offset int
	cursor int

	metric   Metric
	rows     []DisplayRow
	lastSnap live.Snapshot
	errMsg   string
	ready    bool

	detailOpen    bool
	detailLoading bool
	detailBody    string
}

// NewModel builds the interactive model.
func NewModel(ctx context.Context, cfg Config, snap SnapshotFn, recs RecsFn, owner OwnerFn) Model {
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
		owner:    owner,
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

func (m Model) detailCmd(row DisplayRow) tea.Cmd {
	return func() tea.Msg {
		if m.owner == nil {
			return detailMsg{body: FormatOwnerDetail(row.Namespace, row.Pod, live.OwnerTemplate{
				Unsupported: true, UnsupportedReason: "owner lookup not configured",
			}, nil)}
		}
		tmpl, err := m.owner(m.ctx, row.Namespace, row.OwnerKind, row.OwnerName)
		if err != nil {
			return detailMsg{err: err}
		}
		liveRows := live.LiveContainerResources(m.lastSnap.Containers, row.Namespace, row.Pod)
		return detailMsg{body: FormatOwnerDetail(row.Namespace, row.Pod, tmpl, liveRows)}
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
		if m.detailOpen {
			return m, tickCmd(m.cfg.Refresh)
		}
		return m, tea.Batch(m.refreshCmd(), tickCmd(m.cfg.Refresh))
	case snapMsg:
		return m.applySnap(msg), nil
	case detailMsg:
		m.detailLoading = false
		m.detailOpen = true
		if msg.err != nil {
			m.detailBody = "error: " + msg.err.Error() + "\n\n[esc] close\n"
		} else {
			m.detailBody = msg.body
		}
		return m, nil
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
	m.lastSnap = msg.snap
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
	if m.detailOpen {
		return m.handleDetailKey(msg)
	}
	return m.handleListKey(msg)
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.detailOpen = false
		m.detailBody = ""
		m.detailLoading = false
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "enter":
		return m.openDetail()
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
		m.metric = 1 - m.metric
		return m, m.refreshCmd()
	case "b":
		m.cfg.Baseline = 1 - m.cfg.Baseline
		return m, m.refreshCmd()
	case "d":
		m.cfg.ShowDelta = !m.cfg.ShowDelta
	}
	return m, nil
}

func (m Model) openDetail() (tea.Model, tea.Cmd) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return m, nil
	}
	m.detailLoading = true
	m.detailOpen = true
	m.detailBody = "loading owner template…"
	return m, m.detailCmd(m.rows[m.cursor])
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
	h := m.height - 3
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the TUI.
func (m Model) View() string {
	if m.detailOpen {
		return m.renderDetail()
	}
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
		idText := fmt.Sprintf("%-*s", idWidth, FormatIdentity(r.Namespace, r.Pod, idWidth))
		valueText := fmt.Sprintf("%4s", FormatPct(r.Ratio, r.HasRatio))
		if m.cfg.ShowDelta {
			valueText += fmt.Sprintf("  Δcpu %6s  Δmem %7s", FormatCPUDelta(r.Delta), FormatMemDelta(r.Delta))
		}
		if i == m.cursor {
			// Highlight the whole row, not just the gutter: with the identity, bar
			// and value spread across a wide terminal, ">" alone does not say which
			// bar belongs to this row. Segments are styled individually because the
			// bar's own SGR reset would cancel a background wrapped around the line.
			b.WriteString(fmt.Sprintf("%s %s %s %s",
				styleCursor.Render(cur),
				styleRowSel.Render(idText),
				RenderBarOn(r.Ratio, r.HasRatio, barWidth, selBackground),
				styleRowSel.Render(valueText),
			))
		} else {
			b.WriteString(fmt.Sprintf("%s %s %s %s",
				cur, idText, RenderBar(r.Ratio, r.HasRatio, barWidth), valueText))
		}
		b.WriteByte('\n')
	}
	if len(m.rows) == 0 {
		b.WriteString("(no pods)\n")
	}
	return b.String()
}

// renderDetail never returns an empty frame (avoids blank alt-screen).
func (m Model) renderDetail() string {
	ver := strings.TrimSpace(m.cfg.Version)
	if ver == "" {
		ver = "dev"
	}
	body := strings.TrimSpace(m.detailBody)
	if body == "" {
		if m.detailLoading {
			body = "loading owner template…"
		} else {
			body = "(no detail — press esc)"
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("kpm %s  owner template vs pod live (read-only)\n", ver))
	b.WriteString(strings.Repeat("─", 72))
	b.WriteByte('\n')
	b.WriteString(body)
	if !strings.Contains(body, "[esc]") {
		b.WriteString("\n\n[esc] close\n")
	}
	return b.String()
}
