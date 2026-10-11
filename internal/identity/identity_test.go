package identity

import (
	"os"
	"testing"
)

func TestResolveEnvPrecedence(t *testing.T) {
	t.Setenv(EnvID, "from-env")
	got := Resolve("from-config")
	if got != "from-env" {
		t.Fatalf("env should win, got %q", got)
	}
}

func TestResolveConfigFallback(t *testing.T) {
	t.Setenv(EnvID, "")
	got := Resolve("from-config")
	if got != "from-config" {
		t.Fatalf("config should win when env empty, got %q", got)
	}
}

func TestResolveNeverEmpty(t *testing.T) {
	t.Setenv(EnvID, "")
	got := Resolve("")
	if got == "" {
		t.Fatal("resolve must never return empty")
	}
}

func TestResolveTrimsWhitespace(t *testing.T) {
	t.Setenv(EnvID, "   ")
	if got := Resolve("  from-config  "); got != "from-config" {
		t.Fatalf("whitespace-only env must fall through, got %q", got)
	}
}

func TestResolveHostnameFallback(t *testing.T) {
	t.Setenv(EnvID, "   ")
	got := Resolve("  ")
	host, err := os.Hostname()
	if err == nil && host != "" {
		if got != host {
			t.Fatalf("expected hostname %q, got %q", host, got)
		}
		return
	}
	if got == "" {
		t.Fatal("resolve must never return an empty identity")
	}
}

func TestPrimaryHelpersSmoke(t *testing.T) {
	// Exercise network helpers for coverage; values are host-dependent.
	_ = primaryIPv4()
	_ = primaryMAC()
}
