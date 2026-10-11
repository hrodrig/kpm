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

// RenderBar returns a fixed-width heatmap bar (filled/empty) with band color.
func RenderBar(ratio float64, ok bool, width int) string {
	if width < 4 {
		width = 4
	}
	if !ok {
		return styleDim.Render(strings.Repeat("░", width))
	}
	filled := int(ratio * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	switch RatioBand(ratio, true) {
	case BandGreen:
		return styleGreen.Render(bar)
	case BandYellow:
		return styleYellow.Render(bar)
	default:
		return styleRed.Render(bar)
	}
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
