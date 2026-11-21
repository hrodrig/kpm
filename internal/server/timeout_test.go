package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/server"
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
