package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hrodrig/kpm/internal/server"
)

func TestMountUIIndexAndStatic(t *testing.T) {
	mux := http.NewServeMux()
	server.MountUI(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("index %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "analyze-local") || !strings.Contains(body, "/static/app.js") {
		t.Fatalf("body missing markers: %s", body[:min(200, len(body))])
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("css %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "--accent") {
		t.Fatal("css missing tokens")
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/app.js", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("js %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/api/v1/series") {
		t.Fatal("js missing series fetch")
	}
}

func TestMountAllIncludesUI(t *testing.T) {
	mux := http.NewServeMux()
	server.MountAll(mux, &server.Deps{Version: "v-test"}, "")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ui %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("api %d", rr.Code)
	}
	if !server.UIPaths("/") || !server.UIPaths("/static/x") || server.UIPaths("/api/v1/status") {
		t.Fatal("UIPaths")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
