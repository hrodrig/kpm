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
		t.Fatalf("index missing assets: %s", body[:min(200, len(body))])
	}
	if !strings.Contains(body, "theme-toggle") {
		t.Fatal("index missing theme-toggle")
	}
	if !strings.Contains(body, "cluster-target") {
		t.Fatal("index missing cluster-target")
	}
	if !strings.Contains(body, "recommend-summary") {
		t.Fatal("index missing recommend-summary")
	}
	if !strings.Contains(body, "mode-label") {
		t.Fatal("index missing mode-label")
	}
	if !strings.Contains(body, "site-footer") ||
		!strings.Contains(body, "https://github.com/hrodrig/kpm") ||
		!strings.Contains(body, "MIT License") ||
		!strings.Contains(body, "advisory only") {
		t.Fatal("index missing footer / repo / license / disclaimer")
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("css %d", rr.Code)
	}
	css := rr.Body.String()
	if !strings.Contains(css, "--accent") || !strings.Contains(css, `data-theme="dark"`) {
		t.Fatal("css missing tokens/theme")
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/app.js", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("js %d", rr.Code)
	}
	js := rr.Body.String()
	if !strings.Contains(js, "/api/v1/series") {
		t.Fatal("js missing series fetch")
	}
	if !strings.Contains(js, "/api/v1/recommendations") || !strings.Contains(js, "cpu_verdict") {
		t.Fatal("js missing recommendations / verdicts")
	}
	if !strings.Contains(js, "kube_live") || !strings.Contains(js, "updateModeLabel") {
		t.Fatal("js missing kube_live mode label")
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
