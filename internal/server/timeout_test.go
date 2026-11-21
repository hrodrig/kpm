package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/server"
	"github.com/hrodrig/kpm/internal/store"
)

func TestAPILiveEnforcesReadTimeout(t *testing.T) {
	var sawDeadline bool
	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{
		Version:     "v-test",
		ReadTimeout: 40 * time.Millisecond,
		Live: func(ctx context.Context) (server.LiveOut, error) {
			_, sawDeadline = ctx.Deadline()
			<-ctx.Done()
			return server.LiveOut{}, ctx.Err()
		},
	}, "")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/live", nil))
	if !sawDeadline {
		t.Fatal("expected /api/v1/live to attach a context deadline")
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 on live timeout, got %d body=%q", rr.Code, rr.Body.String())
	}
	var out server.LiveOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Available || out.Error == "" {
		t.Fatalf("expected unavailable live timeout JSON, got %+v", out)
	}
}

func TestAPIStoreHandlersSurfaceReadErrors(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	mux := http.NewServeMux()
	server.Mount(mux, &server.Deps{Store: st, Version: "v-test"}, "")

	for _, path := range []string{
		"/api/v1/nodes",
		"/api/v1/recommendations",
		"/api/v1/export/patch?grain_key=Deployment/api/app",
	} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("%s on closed store: want 500, got %d body=%q", path, rr.Code, rr.Body.String())
		}
	}
}
