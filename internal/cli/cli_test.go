package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	Version = "v0.0.0-test"
	Commit = "abc"
	BuildDate = "2026-01-01"
	Branch = "develop"

	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "kpm v0.0.0-test") {
		t.Fatalf("version output missing binary line: %q", out)
	}
	if !strings.Contains(out, "commit: abc") {
		t.Fatalf("version output missing commit: %q", out)
	}
}

func TestPrintSampleConfig(t *testing.T) {
	printSampleConfig = false
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--print-sample-config"})
	err := cmd.Execute()
	if err != nil && !errors.Is(err, errPrintSampleDone) {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "kpm sample configuration") {
		t.Fatalf("sample config missing header: %q", buf.String())
	}
}

func TestExecuteContextBareIsHelp(t *testing.T) {
	printSampleConfig = false
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"kpm"}
	if err := ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteCallsExecuteContext(t *testing.T) {
	printSampleConfig = false
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"kpm", "version"}
	Version = "v0.0.0-test"
	if err := Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteContextPrintSampleFiltersSentinel(t *testing.T) {
	printSampleConfig = false
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"kpm", "--print-sample-config"}
	// Without the filter in ExecuteContext, cobra would return errPrintSampleDone.
	if err := ExecuteContext(t.Context()); err != nil {
		t.Fatalf("ExecuteContext should swallow errPrintSampleDone: %v", err)
	}
}

func TestNotifyTestCommandRegistered(t *testing.T) {
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"notify-test", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "notify-test") || !strings.Contains(buf.String(), "fans out") {
		t.Fatalf("help: %q", buf.String())
	}
}
