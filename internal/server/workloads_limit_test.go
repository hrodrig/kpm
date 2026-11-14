package server_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/server"
	"github.com/hrodrig/kpm/internal/store"
)

func TestAPIWorkloadsScanLimitCommas(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	st, err := store.Open(filepath.Join(t.TempDir(), "big.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// +2: window Until is exclusive at `now`, so a row at exactly `now` is dropped.
	const n = store.MaxScanRows + 2
	batch := make([]store.Record, 0, 2000)
	flush := func() {
		t.Helper()
		if len(batch) == 0 {
			return
		}
		if err := st.InsertRecords(t.Context(), batch); err != nil {
			t.Fatal(err)
		}
		batch = batch[:0]
	}
	for i := 1; i <= n; i++ {
		batch = append(batch, store.Record{
			SampledAt: now.Add(-time.Duration(i) * time.Millisecond),
			Namespace: "app", Pod: "p", Container: "c", OwnerName: "w",
			CPUUtilMcores: 1,
		})
		if len(batch) >= 2000 {
			flush()
		}
	}
	flush()

	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{
		Store: st, Version: "v",
		Now: func() time.Time { return now },
	}, "")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads?window=1h", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 got %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "100,000") {
		t.Fatalf("want comma'd limit in body: %s", rr.Body.String())
	}
}
