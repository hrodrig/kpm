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
	mustContain(t, body, "analyze-local", "/static/theme-init.js", "/static/app.js", "theme-toggle",
		"cluster-target", "recommend-summary", "mode-label", "site-footer",
		"build-meta", "footer-build",
		"https://github.com/hrodrig/kpm", "MIT License", "advisory only",
		"filter-ns", "filter-owner", "filter-grain", "filter-cpu", "filter-mem",
		"Clear filters", "Filter workloads", "nodes-panel", "Node capacity",
		"chrome-sticky", "metric-controls", "data-metric-resource", "data-metric-scale",
		"CPU recommend", "Mem recommend")
	if strings.Contains(body, "<script>") {
		t.Fatal("index.html must not contain inline <script> blocks (breaks strict CSP)")
	}

	themeInit := mustGetBody(t, mux, "/static/theme-init.js", "")
	mustContain(t, themeInit, "kpm_theme", "data-theme")

	css := mustGetBody(t, mux, "/static/app.css", "")
	mustContain(t, css, "--accent", `data-theme="dark"`, ".filters", ".nodes-panel", ".footer-build",
		".chrome-sticky", ".metric-controls", ".badge.pressure", ".near-limit")

	js := mustGetBody(t, mux, "/static/app.js", "")
	mustContain(t, js, "/api/v1/series/batch", "/api/v1/series", "/api/v1/recommendations", "cpu_verdict",
		"kube_live", "updateModeLabel", "filterItems", "workloadQueryParams",
		"No workloads match these filters", "/api/v1/nodes", "record-nodes", "renderNodesPanel",
		"/api/v1/members", "Pods (store)", "formatBuildMeta", "build_date", "go_version",
		"data-container", "recByContainer", "containerRecKey", "formatCount",
		"pressureSignal", "bindMetricControls", "kpm_metric", "above request", "near limit")
}

func TestMountUISecurityHeaders(t *testing.T) {
	mux := http.NewServeMux()
	server.MountUI(mux)

	for _, path := range []string{"/", "/ui", "/static/theme-init.js", "/static/app.css"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status %d", path, rr.Code)
		}
		if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Fatalf("%s X-Frame-Options = %q, want DENY", path, got)
		}
		if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("%s X-Content-Type-Options = %q, want nosniff", path, got)
		}
		if got := rr.Header().Get("Referrer-Policy"); got != "no-referrer" {
			t.Fatalf("%s Referrer-Policy = %q, want no-referrer", path, got)
		}
		csp := rr.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Fatalf("%s Content-Security-Policy = %q", path, csp)
		}
	}
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
