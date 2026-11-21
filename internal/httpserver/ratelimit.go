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

// DefaultMaxRateLimitEntries caps tracked per-IP buckets to prevent unbounded
// memory growth under distributed or spoofed-source floods.
const DefaultMaxRateLimitEntries = 10_000

// RateLimitConfig holds per-IP token-bucket parameters (gghstats mold).
type RateLimitConfig struct {
	Enabled    bool
	Requests   int           // per period (default 120)
	Period     time.Duration // default 1m
	Burst      int           // default 20
	MaxEntries int           // max tracked buckets (default DefaultMaxRateLimitEntries)
}

// DefaultRateLimitConfig returns built-in defaults.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:    true,
		Requests:   120,
		Period:     time.Minute,
		Burst:      20,
		MaxEntries: DefaultMaxRateLimitEntries,
	}
}

// RateLimiter is a per-IP token-bucket rate limiter.
type RateLimiter struct {
	mu           sync.Mutex
	limiters     map[string]*rateLimiterEntry // bucket key → entry
	maxEntries   int
	rate         rate.Limit
	burst        int
	maxIdle      time.Duration
	done         chan struct{}
	shutdownOnce sync.Once
	trusted      *TrustedProxies
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a per-IP limiter and starts idle cleanup.
// Call Shutdown when the server stops.
func NewRateLimiter(cfg RateLimitConfig, trusted *TrustedProxies) *RateLimiter {
	r := rate.Inf
	burst := cfg.Burst
	if cfg.Period <= 0 || cfg.Requests <= 0 || burst <= 0 {
		slog.Warn("rate limit config is non-positive, disabling",
			"requests", cfg.Requests, "period", cfg.Period, "burst", cfg.Burst)
		burst = 1
	} else {
		r = rate.Limit(float64(cfg.Requests) / cfg.Period.Seconds())
	}
	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultMaxRateLimitEntries
	}
	rl := &RateLimiter{
		limiters:   make(map[string]*rateLimiterEntry),
		maxEntries: maxEntries,
		rate:       r,
		burst:      burst,
		maxIdle:    5 * time.Minute,
		done:       make(chan struct{}),
		trusted:    trusted,
	}
	go rl.cleanupLoop()
	return rl
}

// Shutdown stops the background cleanup goroutine.
func (rl *RateLimiter) Shutdown() {
	if rl == nil {
		return
	}
	rl.shutdownOnce.Do(func() {
		close(rl.done)
	})
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
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if e, ok := rl.limiters[key]; ok {
		e.lastSeen = now
		return e
	}
	if len(rl.limiters) >= rl.maxEntries {
		rl.evictIdleLocked(now)
	}
	if len(rl.limiters) >= rl.maxEntries {
		rl.evictOldestLocked()
	}
	e := &rateLimiterEntry{
		limiter:  rate.NewLimiter(rl.rate, rl.burst),
		lastSeen: now,
	}
	rl.limiters[key] = e
	return e
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
	rl.mu.Lock()
	rl.evictIdleLocked(time.Now())
	rl.mu.Unlock()
}

func (rl *RateLimiter) evictIdleLocked(now time.Time) {
	cutoff := now.Add(-rl.maxIdle)
	for key, e := range rl.limiters {
		if e.lastSeen.Before(cutoff) {
			delete(rl.limiters, key)
		}
	}
}

func (rl *RateLimiter) evictOldestLocked() {
	var oldestKey string
	var oldestAt time.Time
	first := true
	for key, e := range rl.limiters {
		if first || e.lastSeen.Before(oldestAt) {
			oldestKey = key
			oldestAt = e.lastSeen
			first = false
		}
	}
	if !first {
		delete(rl.limiters, oldestKey)
	}
}
