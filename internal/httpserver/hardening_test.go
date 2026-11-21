package httpserver

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientIPMultiHeaderXFF(t *testing.T) {
	trusted, err := ParseTrustedProxies("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}

	// Proxy appends a second X-Forwarded-For header line rather than joining
	// onto the client-forged first line. Right-to-left across all lines yields
	// the real client 203.0.113.50, ignoring forged 10.0.0.5.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.RemoteAddr = "192.168.1.10:1234"
	req.Header.Add("X-Forwarded-For", "10.0.0.5")
	req.Header.Add("X-Forwarded-For", "203.0.113.50")
	if got := ClientIP(req, trusted); got != "203.0.113.50" {
		t.Fatalf("multi-header XFF: got %q, want 203.0.113.50", got)
	}

	// Second header contains only trusted proxy hops; walk continues into the
	// first header right-to-left to find the first untrusted client IP.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req2.RemoteAddr = "192.168.1.10:1234"
	req2.Header.Add("X-Forwarded-For", "198.51.100.1, 203.0.113.77")
	req2.Header.Add("X-Forwarded-For", "192.168.1.20")
	if got := ClientIP(req2, trusted); got != "203.0.113.77" {
		t.Fatalf("chained trusted multi-header XFF: got %q, want 203.0.113.77", got)
	}

	// Blank XFF header lines and empty comma-separated hops are skipped cleanly.
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req3.RemoteAddr = "192.168.1.10:1234"
	req3.Header.Add("X-Forwarded-For", "203.0.113.88, , ")
	req3.Header.Add("X-Forwarded-For", "   ")
	if got := ClientIP(req3, trusted); got != "203.0.113.88" {
		t.Fatalf("blank lines/hops XFF: got %q, want 203.0.113.88", got)
	}
}

func TestRateLimiterBoundsAndConcurrentShutdown(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Enabled:    true,
		Requests:   60,
		Period:     time.Minute,
		Burst:      2,
		MaxEntries: 3,
	}, nil)

	// Mark an entry idle beyond maxIdle and verify evictIdle removes it.
	idle := rl.getOrCreate("198.51.100.250")
	rl.mu.Lock()
	idle.lastSeen = time.Now().Add(-10 * time.Minute)
	rl.mu.Unlock()
	rl.evictIdle()
	rl.mu.Lock()
	if _, exists := rl.limiters["198.51.100.250"]; exists {
		rl.mu.Unlock()
		t.Fatal("expected idle entry to be evicted")
	}
	rl.mu.Unlock()

	for i := 1; i <= 10; i++ {
		rl.getOrCreate("198.51.100." + string(rune('0'+i)))
	}
	rl.mu.Lock()
	n := len(rl.limiters)
	rl.mu.Unlock()
	if n > 3 {
		t.Fatalf("rate limiter exceeded maxEntries=3: got %d", n)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Shutdown()
		}()
	}
	wg.Wait()
}

func TestRateLimiterNonPositiveConfigDisablesSafely(t *testing.T) {
	cases := []RateLimitConfig{
		{Enabled: true, Requests: 10, Period: 0, Burst: 5},
		{Enabled: true, Requests: 0, Period: time.Minute, Burst: 5},
		{Enabled: true, Requests: 10, Period: time.Minute, Burst: 0},
	}
	for _, cfg := range cases {
		rl := NewRateLimiter(cfg, nil)
		h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}), PathSkip{})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.RemoteAddr = "203.0.113.1:1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		rl.Shutdown()
		if rr.Code != http.StatusOK {
			t.Fatalf("cfg %+v: want 200, got %d", cfg, rr.Code)
		}
	}
}

func TestAccessControlRateLimitsBeforeAllowlist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	al, err := NewAllowlist([]string{"10.0.0.0/8"}, "real-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	rl := NewRateLimiter(RateLimitConfig{
		Enabled: true, Requests: 2, Period: time.Minute, Burst: 2,
	}, nil)
	defer rl.Shutdown()

	h := AccessControl{Allowlist: al, RateLimit: rl}.Wrap(mux)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.RemoteAddr = "203.0.113.99:1234"
		req.Header.Set("Authorization", "Bearer wrong-guess")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("attempt %d: want 403, got %d", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.RemoteAddr = "203.0.113.99:1234"
	req.Header.Set("Authorization", "Bearer wrong-guess")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("third brute-force attempt must hit rate limit (429), got %d", rr.Code)
	}
}

func TestPathSkipRejectsTraversal(t *testing.T) {
	skip := OpsSkip()
	if !skip.matches("/static/app.js") || !skip.matches("/static/") || !skip.matches("/healthz") {
		t.Fatal("legitimate ops/static paths must match")
	}
	for _, p := range []string{"", "/static/../api/v1/status", "/static/../mcp", "/static/../../etc/passwd"} {
		if skip.matches(p) {
			t.Fatalf("traversal/empty path %q must not match OpsSkip", p)
		}
	}
}

func TestBrowserGuardRejectsZeroEmptyHostAndBadOriginScheme(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := AccessControl{Listen: "127.0.0.1:9740"}.Wrap(mux)

	for _, badHost := range []string{"0.0.0.0", "0.0.0.0:9740", ""} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Host = badHost
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("Host %q: want 403, got %d", badHost, rr.Code)
		}
	}

	for _, badOrigin := range []string{"file://localhost", "javascript://127.0.0.1:9740", "ftp://127.0.0.1"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Host = "127.0.0.1:9740"
		req.Header.Set("Origin", badOrigin)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("Origin %q: want 403, got %d", badOrigin, rr.Code)
		}
	}
}
