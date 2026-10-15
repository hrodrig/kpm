package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSqliteDSNEncodesSpecialPathChars(t *testing.T) {
	dsn := sqliteDSN("/tmp/weird?#name.db")
	if !strings.HasPrefix(dsn, "file:") {
		t.Fatalf("want file: URL, got %q", dsn)
	}
	if !strings.Contains(dsn, "_pragma=busy_timeout") {
		t.Fatalf("missing pragma: %q", dsn)
	}
	if !strings.Contains(dsn, "%3F") || !strings.Contains(dsn, "%23") {
		t.Fatalf("want ?/# escaped in path: %q", dsn)
	}
	if strings.Count(dsn, "?") != 1 {
		t.Fatalf("want single query separator: %q", dsn)
	}
}

func TestOpenModeOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("store file mode %o wants owner-only", fi.Mode().Perm())
	}
}

func TestCloseNilAndPath(t *testing.T) {
	var s *Store
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "m.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Path() != path {
		t.Fatalf("path: %q", st.Path())
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertEmptyAndBadPrune(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.InsertRecords(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PruneRetention(t.Context(), 0); err == nil {
		t.Fatal("expected bad retention")
	}
}

func TestOpenInsertCountPrune(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Second)
	recs := []Record{{
		SampledAt:        now,
		Namespace:        "app",
		Pod:              "web-0",
		Container:        "web",
		CPUUtilMcores:    100,
		MemUtilBytes:     64 << 20,
		CPURequestMcores: 200,
		MemRequestBytes:  128 << 20,
	}}
	if err := s.InsertRecords(t.Context(), recs); err != nil {
		t.Fatal(err)
	}
	n, err := s.Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count: got %d", n)
	}

	old := []Record{{
		SampledAt: now.AddDate(0, 0, -30),
		Namespace: "app",
		Pod:       "old-0",
		Container: "c",
	}}
	if err := s.InsertRecords(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.PruneRetention(t.Context(), 14)
	if err != nil {
		t.Fatal(err)
	}
	if deleted < 1 {
		t.Fatalf("expected prune of old row, deleted=%d", deleted)
	}
	n, err = s.Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("after prune count: got %d", n)
	}
}

func TestListRecords(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UTC()
	recs := []Record{
		{SampledAt: now.Add(-time.Hour), Namespace: "app", Pod: "p", Container: "c", OwnerName: "web", CPUUtilMcores: 1},
		{SampledAt: now, Namespace: "other", Pod: "p", Container: "c", OwnerName: "api", CPUUtilMcores: 2},
	}
	if err := st.InsertRecords(t.Context(), recs); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListRecords(t.Context(), ExportFilter{Namespace: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Namespace != "app" {
		t.Fatalf("got %+v", got)
	}
	got, err = st.ListRecords(t.Context(), ExportFilter{Workload: "api"})
	if err != nil || len(got) != 1 {
		t.Fatalf("workload: %v %#v", err, got)
	}
}

func TestListRecordsLimitNewest(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	var recs []Record
	for i := 0; i < 5; i++ {
		recs = append(recs, Record{
			SampledAt: base.Add(time.Duration(i) * time.Minute),
			Namespace: "app", Pod: "p", Container: "c", OwnerName: "web",
			CPUUtilMcores: float64(i),
		})
	}
	if err := st.InsertRecords(t.Context(), recs); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListRecords(t.Context(), ExportFilter{Namespace: "app", Limit: 2, Newest: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].CPUUtilMcores != 3 || got[1].CPUUtilMcores != 4 {
		t.Fatalf("want newest two ascending: %+v", got)
	}
}

func TestOwnerIndexExists(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var name string
	err = st.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_samples_ns_owner_at'`,
	).Scan(&name)
	if err != nil || name != "idx_samples_ns_owner_at" {
		t.Fatalf("index: %v %q", err, name)
	}
	var ver int
	if err := st.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion {
		t.Fatalf("schema version %d want %d", ver, SchemaVersion)
	}
}

func TestFixedWidthTimestampOrder(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	early := base.Add(100 * time.Nanosecond)
	late := base.Add(900 * time.Nanosecond)
	if err := st.InsertRecords(t.Context(), []Record{
		{SampledAt: late, Namespace: "app", Pod: "p", Container: "c", CPUUtilMcores: 2},
		{SampledAt: early, Namespace: "app", Pod: "p", Container: "c", CPUUtilMcores: 1},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListRecords(t.Context(), ExportFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if !got[0].SampledAt.Equal(early) || !got[1].SampledAt.Equal(late) {
		t.Fatalf("order: %v then %v", got[0].SampledAt, got[1].SampledAt)
	}
	if FormatUTC(early) >= FormatUTC(late) {
		t.Fatalf("lexicographic: %q >= %q", FormatUTC(early), FormatUTC(late))
	}
	// Window edge: until=late exclusive of late itself when equal? until is exclusive <
	win, err := st.ListRecords(t.Context(), ExportFilter{Since: early, Until: late})
	if err != nil {
		t.Fatal(err)
	}
	if len(win) != 1 || !win[0].SampledAt.Equal(early) {
		t.Fatalf("window: %+v", win)
	}
}
