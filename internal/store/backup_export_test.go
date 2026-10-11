package store

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedOne(t *testing.T, st *Store) time.Time {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.InsertRecords(t.Context(), []Record{{
		SampledAt:   now,
		ClientID:    "c1",
		ClusterName: "demo",
		Namespace:   "app",
		Pod:         "web-0",
		Container:   "web",
		OwnerName:   "web",
	}}); err != nil {
		t.Fatal(err)
	}
	return now
}

func TestBackupVACUUMINTO(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "metrics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seedOne(t, st)

	dest := filepath.Join(dir, "backup.db")
	if err := st.Backup(t.Context(), dest); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	n, err := st2.Count(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("backup count=%d err=%v", n, err)
	}
	if err := st.Backup(t.Context(), dest); err == nil {
		t.Fatal("expected exists error")
	}
}

func TestExportCSVAndTSV(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seedOne(t, st)

	var buf bytes.Buffer
	rows, err := st.Export(t.Context(), &buf, FormatCSV, ExportFilter{Namespace: "app"})
	if err != nil || rows != 1 {
		t.Fatalf("csv rows=%d err=%v", rows, err)
	}
	if !strings.Contains(buf.String(), "web-0") {
		t.Fatalf("csv: %q", buf.String())
	}

	buf.Reset()
	rows, err = st.Export(t.Context(), &buf, FormatTSV, ExportFilter{Workload: "web"})
	if err != nil || rows != 1 {
		t.Fatalf("tsv rows=%d err=%v", rows, err)
	}
	if !strings.Contains(buf.String(), "\t") {
		t.Fatalf("expected tabs: %q", buf.String())
	}
}

func TestExportFiltersAndFormat(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := seedOne(t, st)

	var buf bytes.Buffer
	rows, err := st.Export(t.Context(), &buf, FormatCSV, ExportFilter{Namespace: "other"})
	if err != nil || rows != 0 {
		t.Fatalf("filter rows=%d err=%v", rows, err)
	}
	if _, err := st.Export(t.Context(), &buf, "json", ExportFilter{}); err == nil {
		t.Fatal("expected bad format")
	}

	buf.Reset()
	rows, err = st.Export(t.Context(), &buf, FormatCSV, ExportFilter{
		Since: now.Add(-time.Hour),
		Until: now.Add(time.Hour),
		Pod:   "web-0",
	})
	if err != nil || rows != 1 {
		t.Fatalf("time/pod filter rows=%d err=%v", rows, err)
	}
	buf.Reset()
	rows, err = st.Export(t.Context(), &buf, FormatCSV, ExportFilter{
		Until: now.Add(-time.Hour),
	})
	if err != nil || rows != 0 {
		t.Fatalf("until past rows=%d err=%v", rows, err)
	}
}

func TestBackupEdgeCases(t *testing.T) {
	var nilStore *Store
	if err := nilStore.Backup(t.Context(), "x.db"); err == nil {
		t.Fatal("expected not open")
	}
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Backup(t.Context(), ""); err == nil {
		t.Fatal("expected empty dest")
	}
}
