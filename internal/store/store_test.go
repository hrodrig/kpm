package store

import (
	"path/filepath"
	"testing"
	"time"
)

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
