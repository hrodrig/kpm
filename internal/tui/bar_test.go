package tui

import "testing"

func TestRatioBand(t *testing.T) {
	if RatioBand(0, false) != BandNA {
		t.Fatal("na")
	}
	if RatioBand(0.5, true) != BandGreen {
		t.Fatal("green")
	}
	if RatioBand(0.8, true) != BandYellow {
		t.Fatal("yellow")
	}
	if RatioBand(0.95, true) != BandRed {
		t.Fatal("red")
	}
}

func TestRenderBarAndPct(t *testing.T) {
	if FormatPct(0.45, true) != "45%" {
		t.Fatalf("pct: %s", FormatPct(0.45, true))
	}
	if FormatPct(0, false) != "n/a" {
		t.Fatal("na pct")
	}
	bar := RenderBar(0.5, true, 10)
	if bar == "" {
		t.Fatal("empty bar")
	}
	na := RenderBar(0, false, 10)
	if na == "" {
		t.Fatal("empty na bar")
	}
}
