package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/store"
)

func writeMiniConfig(t *testing.T, dir, dbPath string) string {
	t.Helper()
	path := filepath.Join(dir, "kpm.yaml")
	body := `
cluster:
  name: t
client:
  id: test
store:
  path: ` + dbPath + `
  retention_days: 14
sample:
  interval_seconds: 0
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBackupAndExportCommands(t *testing.T) {
	printSampleConfig = false
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")
	cfgPath := writeMiniConfig(t, dir, dbPath)

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertRecords(t.Context(), []store.Record{{
		SampledAt: time.Now().UTC(),
		Namespace: "app",
		Pod:       "p",
		Container: "c",
		ClientID:  "test",
	}}); err != nil {
		t.Fatal(err)
	}
	st.Close()

	bak := filepath.Join(dir, "out.db")
	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"backup", "--config", cfgPath, "-o", bak})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), bak) {
		t.Fatalf("backup out: %q", out.String())
	}

	cmd = NewRootCmd()
	out = &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"export", "--config", cfgPath, "--format", "csv", "-o", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sampled_at") {
		t.Fatalf("export: %q", out.String())
	}
}
