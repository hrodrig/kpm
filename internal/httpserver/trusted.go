package httpserver

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// TrustedProxies is the set of TCP peers allowed to supply X-Forwarded-For / X-Real-IP.
type TrustedProxies struct {
	nets []*net.IPNet
}

// ParseTrustedProxies parses comma-separated or list-style IPs/CIDRs.
// Bare IPv4 → /32; bare IPv6 → /128.
// Empty input returns a non-nil empty set (ContainsIP always false).
//
// A malformed entry is an error, not a skip. Dropping it silently is how a
// typo hides the proxy: rate-limit and allowlist then see the proxy's own
// address, and an allowlist that covers that address admits every client
// behind it. Failing at startup matches what the operator wrote.
func ParseTrustedProxies(entries ...string) (*TrustedProxies, error) {
	tp := &TrustedProxies{}
	for _, raw := range entries {
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
				return nil, fmt.Errorf("invalid trusted_proxies entry %q: %w", strings.TrimSpace(part), err)
			}
			tp.nets = append(tp.nets, n)
		}
	}
	return tp, nil
}

// ContainsIP reports whether ip is inside any trusted network.
func (t *TrustedProxies) ContainsIP(ip net.IP) bool {
	if t == nil || ip == nil {
		return false
	}
	for _, n := range t.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Configured reports whether at least one valid trusted proxy network exists.
func (t *TrustedProxies) Configured() bool {
	return t != nil && len(t.nets) > 0
}

func (t *TrustedProxies) empty() bool {
	return t == nil || len(t.nets) == 0
}

// ShouldWarnTrustedProxies reports whether startup should warn that client IP
// headers are ignored because no trusted proxies were configured.
func ShouldWarnTrustedProxies(trusted *TrustedProxies, rateLimitEnabled, allowlistActive bool) bool {
	if !rateLimitEnabled && !allowlistActive {
		return false
	}
	return trusted.empty()
}

// WarnTrustedProxiesIfNeeded emits a startup warning when middleware depends on
// client IP headers but trusted_proxies is empty.
func WarnTrustedProxiesIfNeeded(trusted *TrustedProxies, rateLimitEnabled, allowlistActive bool) {
	if !ShouldWarnTrustedProxies(trusted, rateLimitEnabled, allowlistActive) {
		return
	}
	slog.Warn("Client IP headers (X-Forwarded-For / X-Real-IP) are ignored because mcp.trusted_proxies is empty. Behind a reverse proxy, set trusted_proxies to the proxy IP or CIDR so rate-limit and allowlist see the real client")
}
