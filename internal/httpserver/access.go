package httpserver

import "net/http"

// AccessControl is the gghstats-style outer middleware stack for the daemon mux.
// Order (outermost first): allowlist → rate limit → mux.
type AccessControl struct {
	Trusted   *TrustedProxies
	Allowlist *Allowlist
	RateLimit *RateLimiter
}

// Wrap returns next with access middleware applied. Ops paths stay exempt.
func (ac AccessControl) Wrap(next http.Handler) http.Handler {
	skip := OpsSkip()
	h := next
	if ac.RateLimit != nil {
		h = ac.RateLimit.Middleware(h, skip)
	}
	if ac.Allowlist != nil {
		h = ac.Allowlist.Middleware(h, skip)
	}
	return h
}

// Active reports whether any IP-keyed middleware is enabled.
func (ac AccessControl) Active() (rateLimit, allowlist bool) {
	return ac.RateLimit != nil, ac.Allowlist != nil && ac.Allowlist.Active()
}
