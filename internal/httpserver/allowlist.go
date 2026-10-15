package httpserver

import (
	"net"
	"net/http"
	"strings"
)

// Allowlist restricts access by client IP/CIDR (gghstats whitelist mold).
type Allowlist struct {
	cidrs   []*net.IPNet
	bearer  string // when set, matching Authorization: Bearer bypasses IP check
	trusted *TrustedProxies
}

// NewAllowlist parses CIDR/IP entries. Returns nil when none are valid (disabled).
// bearer, when non-empty, lets Authorization: Bearer <token> bypass the IP check
// (MCP routes remain gated by BearerAuth).
func NewAllowlist(cidrs []string, bearer string, trusted *TrustedProxies) *Allowlist {
	a := &Allowlist{bearer: strings.TrimSpace(bearer), trusted: trusted}
	for _, raw := range cidrs {
		for _, part := range strings.Split(raw, ",") {
			cidr := strings.TrimSpace(part)
			if cidr == "" {
				continue
			}
			if !strings.Contains(cidr, "/") {
				if strings.Contains(cidr, ":") {
					cidr += "/128"
				} else {
					cidr += "/32"
				}
			}
			_, n, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			a.cidrs = append(a.cidrs, n)
		}
	}
	if len(a.cidrs) == 0 {
		return nil
	}
	return a
}

// Active reports whether the allowlist will enforce IP checks.
func (a *Allowlist) Active() bool { return a != nil && len(a.cidrs) > 0 }

// Middleware only allows requests from allowlisted IPs. Ops paths always pass.
func (a *Allowlist) Middleware(next http.Handler, skip PathSkip) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skip.matches(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if a.bearer != "" && constantTimeEqualString(r.Header.Get("Authorization"), "Bearer "+a.bearer) {
			next.ServeHTTP(w, r)
			return
		}
		ip := ClientIP(r, a.trusted)
		if !a.allowed(ip) {
			http.Error(w, "forbidden\n", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Allowlist) allowed(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range a.cidrs {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}
