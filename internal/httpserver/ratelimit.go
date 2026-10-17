package httpserver

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// bucketPrefixV6 is the IPv6 prefix length used to group rate-limit buckets.
// 64 is the standard subscriber allocation, so a client handed a routed /64 has
// 2^64 source addresses at its disposal.
const bucketPrefixV6 = 64

// bucketKey normalises an address for rate-limit bucketing.
//
// IPv4 and IPv4-mapped IPv6 stay per-address (and normalise to the same key, so
// ::ffff:a.b.c.d cannot buy a second bucket). A real IPv6 address is grouped by
// its /64 network: keying on the full /128 lets a client with a routed /64
// rotate the source address and receive a fresh bucket on every request, which
// makes the limit decorative.
//
// This deliberately does NOT apply to the allowlist. Grouping there would widen
// an operator's intent — a /128 entry would start admitting a whole /64 — so the
// allowlist keeps matching exactly the CIDR that was written.
func bucketKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	mask := net.CIDRMask(bucketPrefixV6, 128)
	return (&net.IPNet{IP: parsed.Mask(mask), Mask: mask}).String()
}

// RateLimitConfig holds per-IP token-bucket parameters (gghstats mold).
type RateLimitConfig struct {
	Enabled  bool
	Requests int           // per period (default 120)
	Period   time.Duration // default 1m
	Burst    int           // default 20
}

// DefaultRateLimitConfig returns built-in defaults.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:  true,
		Requests: 120,
		Period:   time.Minute,
		Burst:    20,
	}
}

// RateLimiter is a per-IP token-bucket rate limiter.
type RateLimiter struct {
	limiters sync.Map // bucket key → *rateLimiterEntry
	rate     rate.Limit
	burst    int
	maxIdle  time.Duration
	done     chan struct{}
	trusted  *TrustedProxies
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	mu       sync.Mutex
	lastSeen time.Time
}

// NewRateLimiter creates a per-IP limiter and starts idle cleanup.
// Call Shutdown when the server stops.
func NewRateLimiter(cfg RateLimitConfig, trusted *TrustedProxies) *RateLimiter {
	r := rate.Limit(float64(cfg.Requests) / cfg.Period.Seconds())
	if r <= 0 {
		slog.Warn("rate limit requests/period is zero or negative, disabling",
			"requests", cfg.Requests, "period", cfg.Period)
		r = rate.Inf
	}
	rl := &RateLimiter{
		rate:    r,
		burst:   cfg.Burst,
		maxIdle: 5 * time.Minute,
		done:    make(chan struct{}),
		trusted: trusted,
	}
	go rl.cleanupLoop()
	return rl
}

// Shutdown stops the background cleanup goroutine.
func (rl *RateLimiter) Shutdown() {
	if rl == nil {
		return
	}
	select {
	case <-rl.done:
	default:
		close(rl.done)
	}
}

// Middleware wraps next with per-IP rate limiting. Paths in skip are exempt.
func (rl *RateLimiter) Middleware(next http.Handler, skip PathSkip) http.Handler {
	if rl == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skip.matches(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ip := ClientIP(r, rl.trusted)
		entry := rl.getOrCreate(ip)
		if !entry.limiter.Allow() {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}` + "\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) getOrCreate(ip string) *rateLimiterEntry {
	key := bucketKey(ip)
	if v, ok := rl.limiters.Load(key); ok {
		e := v.(*rateLimiterEntry)
		e.mu.Lock()
		e.lastSeen = time.Now()
		e.mu.Unlock()
		return e
	}
	e := &rateLimiterEntry{
		limiter:  rate.NewLimiter(rl.rate, rl.burst),
		lastSeen: time.Now(),
	}
	actual, _ := rl.limiters.LoadOrStore(key, e)
	got := actual.(*rateLimiterEntry)
	if got != e {
		got.mu.Lock()
		got.lastSeen = time.Now()
		got.mu.Unlock()
	}
	return got
}

func (rl *RateLimiter) cleanupLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-rl.done:
			return
		case <-t.C:
			rl.evictIdle()
		}
	}
}

func (rl *RateLimiter) evictIdle() {
	cutoff := time.Now().Add(-rl.maxIdle)
	rl.limiters.Range(func(key, value any) bool {
		e := value.(*rateLimiterEntry)
		e.mu.Lock()
		idle := e.lastSeen.Before(cutoff)
		e.mu.Unlock()
		if idle {
			rl.limiters.Delete(key)
		}
		return true
	})
}
