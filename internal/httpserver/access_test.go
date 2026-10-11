package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/httpserver"
)

func TestAllowlistAndOpsExempt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	trusted := httpserver.ParseTrustedProxies()
	al := httpserver.NewAllowlist([]string{"10.0.0.0/8"}, "", trusted)
	h := httpserver.AccessControl{Trusted: trusted, Allowlist: al}.Wrap(mux)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "203.0.113.1:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz exempt: %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.RemoteAddr = "203.0.113.1:1234"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("sse blocked: %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.RemoteAddr = "10.1.2.3:9"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("sse allowlisted: %d", rr.Code)
	}
}

func TestAllowlistBearerBypass(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	trusted := httpserver.ParseTrustedProxies()
	al := httpserver.NewAllowlist([]string{"10.0.0.0/8"}, "tok", trusted)
	h := httpserver.AccessControl{Allowlist: al}.Wrap(mux)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.RemoteAddr = "203.0.113.1:1"
	req.Header.Set("Authorization", "Bearer tok")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bearer bypass: %d", rr.Code)
	}
}

func TestTrustedProxiesXFF(t *testing.T) {
	trusted := httpserver.ParseTrustedProxies("192.168.1.0/24")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.10:1"
	req.Header.Set("X-Forwarded-For", "10.5.5.5")
	if got := httpserver.ClientIP(req, trusted); got != "10.5.5.5" {
		t.Fatalf("trusted XFF: %q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "203.0.113.9:1"
	req2.Header.Set("X-Forwarded-For", "10.5.5.5")
	if got := httpserver.ClientIP(req2, trusted); got != "203.0.113.9" {
		t.Fatalf("untrusted forge ignored: %q", got)
	}
}

func TestRateLimitBlocks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	trusted := httpserver.ParseTrustedProxies()
	rl := httpserver.NewRateLimiter(httpserver.RateLimitConfig{
		Enabled: true, Requests: 2, Period: time.Minute, Burst: 2,
	}, trusted)
	defer rl.Shutdown()
	h := httpserver.AccessControl{Trusted: trusted, RateLimit: rl}.Wrap(mux)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/sse", nil)
		req.RemoteAddr = "203.0.113.1:1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("req %d: %d", i, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.RemoteAddr = "203.0.113.1:1"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "203.0.113.1:1"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz still ok: %d", rr.Code)
	}
}

func TestShouldWarnTrustedProxies(t *testing.T) {
	empty := httpserver.ParseTrustedProxies()
	if !httpserver.ShouldWarnTrustedProxies(empty, true, false) {
		t.Fatal("want warn when rate limit on")
	}
	if httpserver.ShouldWarnTrustedProxies(httpserver.ParseTrustedProxies("10.0.0.0/8"), true, true) {
		t.Fatal("no warn when configured")
	}
	if httpserver.ShouldWarnTrustedProxies(empty, false, false) {
		t.Fatal("no warn when idle")
	}
	httpserver.WarnTrustedProxiesIfNeeded(empty, true, false)
	if empty.Configured() {
		t.Fatal("empty must not be Configured")
	}
	if !httpserver.ParseTrustedProxies("127.0.0.1").Configured() {
		t.Fatal("want configured")
	}
}

func TestServerSetAccessControl(t *testing.T) {
	s := httpserver.New("127.0.0.1:0", nil, nil)
	trusted := httpserver.ParseTrustedProxies("10.0.0.0/8")
	al := httpserver.NewAllowlist([]string{"10.0.0.0/8"}, "", trusted)
	rl := httpserver.NewRateLimiter(httpserver.DefaultRateLimitConfig(), trusted)
	defer rl.Shutdown()
	ac := httpserver.AccessControl{Trusted: trusted, Allowlist: al, RateLimit: rl}
	s.SetAccessControl(ac)
	rlOn, alOn := s.Access().Active()
	if !rlOn || !alOn || !al.Active() {
		t.Fatalf("active: rl=%v al=%v", rlOn, alOn)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "203.0.113.1:1"
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ops exempt via Server: %d", rr.Code)
	}
}

func TestClientIPRealIPAndPeer(t *testing.T) {
	trusted := httpserver.ParseTrustedProxies("192.168.0.0/16", "::1")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:9"
	req.Header.Set("X-Real-IP", "203.0.113.50")
	if got := httpserver.ClientIP(req, trusted); got != "203.0.113.50" {
		t.Fatalf("X-Real-IP: %q", got)
	}
	// Invalid XFF falls through to peer.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.1:9"
	req2.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := httpserver.ClientIP(req2, trusted); got != "192.168.1.1" {
		t.Fatalf("bad XFF peer: %q", got)
	}
	// Bare RemoteAddr without port.
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "10.0.0.1"
	if got := httpserver.ClientIP(req3, httpserver.ParseTrustedProxies()); got != "10.0.0.1" {
		t.Fatalf("bare peer: %q", got)
	}
}

func TestNewAllowlistEdges(t *testing.T) {
	if httpserver.NewAllowlist(nil, "", nil) != nil {
		t.Fatal("empty allowlist")
	}
	if httpserver.NewAllowlist([]string{"not-a-cidr", "bogus"}, "", nil) != nil {
		t.Fatal("all invalid")
	}
	a := httpserver.NewAllowlist([]string{"2001:db8::1"}, "", nil)
	if a == nil || !a.Active() {
		t.Fatal("want ipv6 /128")
	}
	// Prefix skip path.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	skip := httpserver.PathSkip{Prefixes: []string{"/met"}}
	h := a.Middleware(mux, skip)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.1:1"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("prefix skip: %d", rr.Code)
	}
}
