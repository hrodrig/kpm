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

	body := mustGetBody(t, mux, "/", "text/html")
	mustContain(t, body, "analyze-local", "/static/app.js", "theme-toggle",
		"cluster-target", "recommend-summary", "mode-label", "site-footer",
		"https://github.com/hrodrig/kpm", "MIT License", "advisory only")

	css := mustGetBody(t, mux, "/static/app.css", "")
	mustContain(t, css, "--accent", `data-theme="dark"`)

	js := mustGetBody(t, mux, "/static/app.js", "")
	mustContain(t, js, "/api/v1/series/batch", "/api/v1/series", "/api/v1/recommendations", "cpu_verdict",
		"kube_live", "updateModeLabel")
}

func TestMountAllIncludesUI(t *testing.T) {
	mux := http.NewServeMux()
	server.MountAll(mux, &server.Deps{Version: "v-test"}, "")
	mustStatus(t, mux, "/", http.StatusOK)
	mustStatus(t, mux, "/api/v1/status", http.StatusOK)
	if !server.UIPaths("/") || !server.UIPaths("/static/x") || server.UIPaths("/api/v1/status") {
		t.Fatal("UIPaths")
	}
}

func mustGetBody(t *testing.T, h http.Handler, path, wantCT string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("%s status %d", path, rr.Code)
	}
	if wantCT != "" && !strings.Contains(rr.Header().Get("Content-Type"), wantCT) {
		t.Fatalf("%s content-type %q", path, rr.Header().Get("Content-Type"))
	}
	return rr.Body.String()
}

func mustStatus(t *testing.T, h http.Handler, path string, code int) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if rr.Code != code {
		t.Fatalf("%s want %d got %d", path, code, rr.Code)
	}
}

func mustContain(t *testing.T, hay string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(hay, n) {
			snip := hay
			if len(snip) > 200 {
				snip = snip[:200]
			}
			t.Fatalf("missing %q in: %s", n, snip)
		}
	}
}
