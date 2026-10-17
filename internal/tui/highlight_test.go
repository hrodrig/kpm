package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func readyModel(t *testing.T) Model {
	t.Helper()
	updated, _ := newTestModel().Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := updated.(Model)
	updated, _ = m.Update(snapMsg{snap: sampleSnap(), idx: RecIndex{}})
	m = updated.(Model)
	if len(m.rows) != 2 {
		t.Fatalf("rows: %d", len(m.rows))
	}
	return m
}

// The selected row has to be identifiable across its whole width, not only by
// the ">" gutter: on a wide terminal the bar and the value sit far from the name.
func TestSelectedRowIsHighlightedAcrossTheRow(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)

	m := readyModel(t)
	rendered := m.View()

	var selected, unselected string
	for _, line := range strings.Split(rendered, "\n") {
		switch {
		case strings.Contains(line, ">"):
			selected = line
		case strings.Contains(line, "p1") || strings.Contains(line, "p2"):
			if unselected == "" {
				unselected = line
			}
		}
	}
	if selected == "" || unselected == "" {
		t.Fatalf("could not identify both rows in render:\n%s", rendered)
	}

	const background = "\x1b[48;5;24m"
	if !strings.Contains(selected, background) {
		t.Fatalf("selected row carries no background highlight:\n%q", selected)
	}
	if strings.Contains(unselected, background) {
		t.Fatalf("unselected row is highlighted too:\n%q", unselected)
	}
}

// The highlight must not move anything: the columns would jitter as the cursor
// moves down the list.
func TestRenderBarOnKeepsBarWidth(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)

	const width = 20
	plain := lipgloss.Width(RenderBar(0.5, true, width))
	highlighted := lipgloss.Width(RenderBarOn(0.5, true, width, selBackground))

	if plain != highlighted {
		t.Fatalf("bar width changed with the highlight: plain=%d highlighted=%d", plain, highlighted)
	}
	if highlighted != width {
		t.Fatalf("bar width: got %d want %d", highlighted, width)
	}
}

// A selected row must not disturb the plain-text layout when colour is stripped
// (pipes, tests, dumb terminals): same columns, only the gutter differs.
func TestSelectedRowKeepsPlainLayout(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(prev)

	m := readyModel(t)
	lines := strings.Split(m.View(), "\n")

	if len(lines) < 3 {
		t.Fatalf("expected header + 2 rows, got %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	first, second := lines[1], lines[2]
	if len(first) != len(second) {
		t.Fatalf("rows differ in width: %d vs %d\n%q\n%q", len(first), len(second), first, second)
	}
	if !strings.HasPrefix(first, ">") {
		t.Fatalf("cursor row should start with '>': %q", first)
	}
	if !strings.HasPrefix(second, " ") {
		t.Fatalf("non-cursor row should start with a space: %q", second)
	}
}
