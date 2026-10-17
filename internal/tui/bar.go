package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Band is a heatmap color band for util ratio vs baseline.
type Band int

const (
	BandGreen Band = iota
	BandYellow
	BandRed
	BandNA
)

// RatioBand maps ratio to color band (green <0.70, yellow <0.90, else red).
func RatioBand(ratio float64, ok bool) Band {
	if !ok {
		return BandNA
	}
	switch {
	case ratio < 0.70:
		return BandGreen
	case ratio < 0.90:
		return BandYellow
	default:
		return BandRed
	}
}

var (
	styleGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Selected-row styling.
//
// The row background is a dark cyan, not a bright one: the bars carry
// green/yellow/red, and those stay readable on a dark field but wash out on a
// saturated one. The bright accent lives in the cursor gutter instead.
var (
	selBackground = lipgloss.Color("24")
	styleRowSel   = lipgloss.NewStyle().Background(selBackground)
	styleCursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

// bandStyle is the foreground style for a ratio.
func bandStyle(ratio float64, ok bool) lipgloss.Style {
	if !ok {
		return styleDim
	}
	switch RatioBand(ratio, true) {
	case BandGreen:
		return styleGreen
	case BandYellow:
		return styleYellow
	default:
		return styleRed
	}
}

// RenderBar returns a fixed-width heatmap bar (filled/empty) with band color.
func RenderBar(ratio float64, ok bool, width int) string {
	return renderBar(ratio, ok, width, lipgloss.NewStyle())
}

// RenderBarOn is RenderBar carrying a background, used by the selected row.
//
// A highlighted row cannot be produced by styling the finished line: RenderBar
// emits its own SGR reset, which cancels any background wrapped around it, so
// every segment of a highlighted row has to carry the background itself.
func RenderBarOn(ratio float64, ok bool, width int, bg lipgloss.Color) string {
	return renderBar(ratio, ok, width, lipgloss.NewStyle().Background(bg))
}

func renderBar(ratio float64, ok bool, width int, base lipgloss.Style) string {
	if width < 4 {
		width = 4
	}
	if !ok {
		return bandStyle(0, false).Inherit(base).Render(strings.Repeat("░", width))
	}
	filled := int(ratio * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return bandStyle(ratio, true).Inherit(base).Render(bar)
}

// FormatPct formats a ratio as percent or n/a.
func FormatPct(ratio float64, ok bool) string {
	if !ok {
		return "n/a"
	}
	pct := int(ratio*100 + 0.5)
	if pct < 0 {
		pct = 0
	}
	return fmt.Sprintf("%d%%", pct)
}
