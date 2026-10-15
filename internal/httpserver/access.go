package httpserver

import (
	"net/http"
	"strings"
)

// AccessControl is the gghstats-style outer middleware stack for the daemon mux.
// Order (outermost first): browser guard → allowlist → rate limit → mux.
type AccessControl struct {
	Trusted   *TrustedProxies
	Allowlist *Allowlist
	RateLimit *RateLimiter
	// Listen is the bind address (used for loopback Host/Origin checks).
	Listen string
	// AllowedHosts are extra Host/Origin names accepted on loopback listen
	// (reverse-proxy public names). Loopback names are always allowed.
	AllowedHosts []string
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
	if strings.TrimSpace(ac.Listen) != "" {
		h = BrowserGuard(ac.Listen, ac.AllowedHosts, h)
	}
	return h
}

// Active reports whether any IP-keyed middleware is enabled.
func (ac AccessControl) Active() (rateLimit, allowlist bool) {
	return ac.RateLimit != nil, ac.Allowlist != nil && ac.Allowlist.Active()
}
