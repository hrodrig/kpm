package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestBuildExportFilterTimes(t *testing.T) {
	since := "2026-10-01T00:00:00Z"
	until := "2026-10-11T00:00:00Z"
	f, err := buildExportFilter(exportFlags{
		sinceStr:  since,
		untilStr:  until,
		namespace: "app",
		pod:       "p",
		workload:  "w",
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.Namespace != "app" || f.Pod != "p" || f.Workload != "w" {
		t.Fatalf("filters: %+v", f)
	}
	if !f.Since.Equal(mustRFC3339(t, since)) || !f.Until.Equal(mustRFC3339(t, until)) {
		t.Fatalf("times: since=%v until=%v", f.Since, f.Until)
	}
	if _, err := buildExportFilter(exportFlags{sinceStr: "bad"}); err == nil {
		t.Fatal("expected since parse error")
	}
	if _, err := buildExportFilter(exportFlags{untilStr: "bad"}); err == nil {
		t.Fatal("expected until parse error")
	}
}

func TestExportWriterStdoutAndFile(t *testing.T) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	w, c, err := exportWriter(cmd, "-")
	if err != nil || c != nil || w == nil {
		t.Fatalf("stdout writer err=%v closer=%v", err, c)
	}
	path := filepath.Join(t.TempDir(), "out.csv")
	w, c, err = exportWriter(cmd, path)
	if err != nil || c == nil {
		t.Fatalf("file writer err=%v", err)
	}
	_, _ = w.Write([]byte("x"))
	_ = c.Close()
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "x" {
		t.Fatalf("file contents %q err=%v", b, err)
	}
}

func mustRFC3339(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tt
}
