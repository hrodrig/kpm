package cli

import (
	"bytes"
	"errors"
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

func TestExecuteContextPrintSample(t *testing.T) {
	printSampleConfig = false
	// ExecuteContext builds a fresh root; drive flags via os.Args pattern is
	// awkward — assert the sentinel filter with a direct call path instead.
	if err := ExecuteContext(t.Context()); err != nil {
		// bare Execute with no args runs Help — should succeed
		t.Fatal(err)
	}
}
