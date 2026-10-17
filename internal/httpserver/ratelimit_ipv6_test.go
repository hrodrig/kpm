package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// #30: an IPv6 client with a routed /64 can present any of 2^64 source
// addresses. Keying the bucket on the full /128 hands it a fresh bucket per
// request, which makes the limit decorative.
func TestBucketKeyGroupsIPv6By64(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want string
	}{
		{"v6 address", "2001:db8:1:2:3:4:5:6", "2001:db8:1:2::/64"},
		{"v6 other host, same /64", "2001:db8:1:2:ffff::1", "2001:db8:1:2::/64"},
		{"v6 different /64", "2001:db8:1:3::1", "2001:db8:1:3::/64"},
		{"v4 stays per-address", "192.0.2.7", "192.0.2.7"},
		{"v4-mapped normalises to v4", "::ffff:192.0.2.7", "192.0.2.7"},
		{"unparseable is left alone", "not-an-ip", "not-an-ip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bucketKey(tc.ip); got != tc.want {
				t.Fatalf("bucketKey(%q) = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

func TestRateLimiterBucketIdentity(t *testing.T) {
	rl := NewRateLimiter(DefaultRateLimitConfig(), nil)
	defer rl.Shutdown()

	sameA := rl.getOrCreate("2001:db8:1:2::1")
	sameB := rl.getOrCreate("2001:db8:1:2:beef::9")
	if sameA != sameB {
		t.Fatal("addresses inside one /64 must share a bucket")
	}
	if other := rl.getOrCreate("2001:db8:1:3::1"); other == sameA {
		t.Fatal("a different /64 must get its own bucket")
	}
	v4 := rl.getOrCreate("192.0.2.7")
	if other := rl.getOrCreate("192.0.2.8"); other == v4 {
		t.Fatal("distinct IPv4 addresses must not share a bucket")
	}
	if mapped := rl.getOrCreate("::ffff:192.0.2.7"); mapped != v4 {
		t.Fatal("the v4-mapped form must reuse the IPv4 bucket, not buy a second one")
	}
}

// Behavioural check through the middleware: three requests from three different
// source addresses inside one /64, with burst 2, must produce a 429.
func TestRateLimiterMiddlewareCountsRotatingIPv6AsOneClient(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Enabled: true, Requests: 60, Period: time.Minute, Burst: 2}, nil)
	defer rl.Shutdown()
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), PathSkip{})

	addrs := []string{
		"2001:db8:1:2::1:1",
		"2001:db8:1:2::2:2",
		"2001:db8:1:2::3:3",
	}
	var codes []int
	for _, a := range addrs {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
		req.RemoteAddr = "[" + a + "]:54321"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
		t.Fatalf("the first two requests should pass, got %v", codes)
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("rotating the source within one /64 must not refill the bucket, codes=%v", codes)
	}
}

// Control: distinct /64s are distinct clients, so the third request from a fresh
// network must still pass.
func TestRateLimiterMiddlewareSeparatesDistinctIPv6Subnets(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Enabled: true, Requests: 60, Period: time.Minute, Burst: 2}, nil)
	defer rl.Shutdown()
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), PathSkip{})

	addrs := []string{
		"2001:db8:1:2::1",
		"2001:db8:1:2::2",
		"2001:db8:9:9::1", // different /64: its own burst
	}
	var codes []int
	for _, a := range addrs {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
		req.RemoteAddr = "[" + a + "]:54321"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK || codes[2] != http.StatusOK {
		t.Fatalf("a fresh /64 must keep its own budget, codes=%v", codes)
	}
}
