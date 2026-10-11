package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hrodrig/kpm/internal/live"
	"github.com/hrodrig/kpm/internal/store"
)

func sampleSnap() live.Snapshot {
	return live.Snapshot{Containers: []live.ContainerRow{
		{Namespace: "a", Pod: "p1", Container: "c", HasUtil: true,
			CPUUtilMcores: 80, CPURequestMcores: 100, CPULimitMcores: 200,
			MemUtilBytes: 40, MemRequestBytes: 100, MemLimitBytes: 200},
		{Namespace: "a", Pod: "p2", Container: "c", HasUtil: true,
			CPUUtilMcores: 10, CPURequestMcores: 100,
			MemUtilBytes: 10, MemRequestBytes: 100},
	}}
}

func newTestModel() Model {
	snap := sampleSnap()
	return NewModel(context.Background(), Config{
		Refresh:  time.Second,
		Sort:     live.SortCPURatio,
		Baseline: BaselineRequest,
	}, func(context.Context) (live.Snapshot, error) {
		return snap, nil
	}, func(context.Context) ([]store.Recommendation, error) {
		return nil, nil
	})
}

func TestModelSnapAndView(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mod := updated.(Model)
	updated, _ = mod.Update(snapMsg{snap: sampleSnap(), idx: RecIndex{}})
	mod = updated.(Model)
	if len(mod.rows) != 2 {
		t.Fatalf("rows: %d", len(mod.rows))
	}
	mod.cfg.ShowDelta = true
	view := mod.View()
	if !strings.Contains(view, "Δcpu") {
		t.Fatalf("view: %q", view)
	}
}

func TestModelToggles(t *testing.T) {
	m := newTestModel()
	m.ready = true
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	mod := updated.(Model)
	if cmd == nil || mod.metric != MetricMem {
		t.Fatal("metric")
	}
	updated, cmd = mod.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	mod = updated.(Model)
	if cmd == nil || mod.cfg.Baseline != BaselineLimit {
		t.Fatal("baseline")
	}
	updated, cmd = mod.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	mod = updated.(Model)
	if cmd == nil || mod.cfg.Sort != live.SortMemRatio {
		t.Fatalf("sort: %s", mod.cfg.Sort)
	}
	updated, _ = mod.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mod = updated.(Model)
	if !mod.cfg.ShowDelta {
		t.Fatal("delta")
	}
}

func TestModelNavAndQuit(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(snapMsg{snap: sampleSnap(), idx: RecIndex{}})
	mod := updated.(Model)
	mod.height = 24
	updated, _ = mod.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mod = updated.(Model)
	if mod.cursor != 1 {
		t.Fatalf("down: %d", mod.cursor)
	}
	updated, _ = mod.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	mod = updated.(Model)
	if mod.cursor != 0 {
		t.Fatalf("up: %d", mod.cursor)
	}
	_, cmd := mod.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit")
	}
	_, cmd = mod.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("tick")
	}
}

func TestModelSnapErrorsAndInit(t *testing.T) {
	m := NewModel(context.Background(), Config{}, func(context.Context) (live.Snapshot, error) {
		return live.Snapshot{}, errors.New("api down")
	}, func(context.Context) ([]store.Recommendation, error) {
		return nil, errors.New("db")
	})
	if m.Init() == nil {
		t.Fatal("init")
	}
	updated, _ := m.Update(snapMsg{err: errors.New("api down")})
	mod := updated.(Model)
	if mod.errMsg == "" {
		t.Fatal("err msg")
	}
	updated, _ = mod.Update(snapMsg{
		snap: sampleSnap(),
		idx:  IndexRecommendations(nil),
		err:  errors.New("recommendations: db"),
	})
	mod = updated.(Model)
	if len(mod.rows) != 2 || mod.errMsg == "" {
		t.Fatalf("soft err: rows=%d err=%q", len(mod.rows), mod.errMsg)
	}
}

func TestModelEmptyPodsView(t *testing.T) {
	m := newTestModel()
	m.ready = true
	m.width = 80
	m.height = 20
	updated, _ := m.Update(snapMsg{snap: live.Snapshot{}, idx: RecIndex{}})
	mod := updated.(Model)
	if !strings.Contains(mod.View(), "(no pods)") {
		t.Fatalf("view: %q", mod.View())
	}
}
