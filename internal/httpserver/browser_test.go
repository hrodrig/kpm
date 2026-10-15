package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hrodrig/kpm/internal/httpserver"
)

func TestBrowserGuardLoopbackRejectsBadHostOrigin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := httpserver.AccessControl{Listen: "127.0.0.1:9740", AllowedHosts: []string{"kpm.example.com"}}.Wrap(mux)

	// Empty Origin + loopback Host: allowed (curl).
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Host = "127.0.0.1:9740"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("same-origin curl: %d", rr.Code)
	}

	// DNS-rebinding Host.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Host = "evil.example"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("bad Host: %d", rr.Code)
	}

	// Cross-origin browser request.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Host = "127.0.0.1:9740"
	req.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("bad Origin: %d", rr.Code)
	}

	// Legitimate loopback Origin.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Host = "localhost:9740"
	req.Header.Set("Origin", "http://127.0.0.1:9740")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("loopback Origin: %d", rr.Code)
	}

	// Configured public Host behind reverse proxy to loopback.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Host = "kpm.example.com"
	req.Header.Set("Origin", "https://kpm.example.com")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("allowed Host/Origin: %d", rr.Code)
	}

	// Ops paths stay exempt.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h = httpserver.AccessControl{Listen: "127.0.0.1:9740"}.Wrap(mux)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Host = "evil.example"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ops exempt: %d", rr.Code)
	}
}
