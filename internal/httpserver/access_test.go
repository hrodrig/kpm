package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/httpserver"
)

func mustTrusted(t *testing.T, entries ...string) *httpserver.TrustedProxies {
	t.Helper()
	tp, err := httpserver.ParseTrustedProxies(entries...)
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

func TestAllowlistAndOpsExempt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	trusted := mustTrusted(t)
	al, err := httpserver.NewAllowlist([]string{"10.0.0.0/8"}, "", trusted)
	if err != nil {
		t.Fatal(err)
	}
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
	trusted := mustTrusted(t)
	al, err := httpserver.NewAllowlist([]string{"10.0.0.0/8"}, "tok", trusted)
	if err != nil {
		t.Fatal(err)
	}
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
	trusted := mustTrusted(t, "192.168.1.0/24")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.10:1"
	req.Header.Set("X-Forwarded-For", "10.5.5.5")
	if got := httpserver.ClientIP(req, trusted); got != "10.5.5.5" {
		t.Fatalf("trusted XFF: %q", got)
	}

	// Append-style XFF: client forges leftmost; rightmost hop is the proxy.
	// Walk right-to-left skipping trusted → real client 203.0.113.50.
	reqAppend := httptest.NewRequest(http.MethodGet, "/", nil)
	reqAppend.RemoteAddr = "192.168.1.10:1"
	reqAppend.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.50, 192.168.1.10")
	if got := httpserver.ClientIP(reqAppend, trusted); got != "203.0.113.50" {
		t.Fatalf("append XFF: %q", got)
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
	trusted := mustTrusted(t)
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

	mux.HandleFunc("GET /static/app.js", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h = httpserver.AccessControl{Trusted: trusted, RateLimit: rl}.Wrap(mux)
	for i := 0; i < 5; i++ {
		req = httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
		req.RemoteAddr = "203.0.113.1:1"
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("static exempt from rate limit: %d", rr.Code)
		}
	}
}

func TestShouldWarnTrustedProxies(t *testing.T) {
	empty := mustTrusted(t)
	if !httpserver.ShouldWarnTrustedProxies(empty, true, false) {
		t.Fatal("want warn when rate limit on")
	}
	if httpserver.ShouldWarnTrustedProxies(mustTrusted(t, "10.0.0.0/8"), true, true) {
		t.Fatal("no warn when configured")
	}
	if httpserver.ShouldWarnTrustedProxies(empty, false, false) {
		t.Fatal("no warn when idle")
	}
	httpserver.WarnTrustedProxiesIfNeeded(empty, true, false)
	if empty.Configured() {
		t.Fatal("empty must not be Configured")
	}
	if !mustTrusted(t, "127.0.0.1").Configured() {
		t.Fatal("want configured")
	}
}

func TestServerSetAccessControl(t *testing.T) {
	s := httpserver.New("127.0.0.1:0", nil, nil)
	trusted := mustTrusted(t, "10.0.0.0/8")
	al, err := httpserver.NewAllowlist([]string{"10.0.0.0/8"}, "", trusted)
	if err != nil {
		t.Fatal(err)
	}
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
	trusted := mustTrusted(t, "192.168.0.0/16", "::1")
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
	if got := httpserver.ClientIP(req3, mustTrusted(t)); got != "10.0.0.1" {
		t.Fatalf("bare peer: %q", got)
	}
}

func TestNewAllowlistEdges(t *testing.T) {
	if al, err := httpserver.NewAllowlist(nil, "", nil); al != nil || err != nil {
		t.Fatalf("empty allowlist should stay off: al=%v err=%v", al, err)
	}
	// A malformed entry is an error, not a silent skip: skipping is how an operator's
	// only entry becomes an empty list that admits everyone.
	if al, err := httpserver.NewAllowlist([]string{"not-a-cidr", "bogus"}, "", nil); err == nil || al != nil {
		t.Fatalf("invalid entries must fail closed: al=%v err=%v", al, err)
	}
	a, err := httpserver.NewAllowlist([]string{"2001:db8::1"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
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

func TestParseTrustedProxies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []string
		wantErr string
		wantOn  bool
	}{
		{name: "empty stays off", entries: nil},
		{name: "blank parts ignored", entries: []string{"", "  ,  "}},
		{name: "bare ipv4 is /32", entries: []string{"127.0.0.1"}, wantOn: true},
		{name: "cidr and bare ipv6", entries: []string{"10.0.0.0/8, ::1"}, wantOn: true},
		{name: "bad prefix fails closed", entries: []string{"10.0.0.0/33"}, wantErr: "10.0.0.0/33"},
		{name: "typo beside a valid entry fails closed", entries: []string{"10.0.0.0/8", "not-a-cidr"}, wantErr: "not-a-cidr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := httpserver.ParseTrustedProxies(tt.entries...)
			if tt.wantErr != "" {
				if err == nil || got != nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q and nil set, got %v err=%v", tt.wantErr, got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("nil set")
			}
			if got.Configured() != tt.wantOn {
				t.Fatalf("configured=%v want %v", got.Configured(), tt.wantOn)
			}
		})
	}
}
