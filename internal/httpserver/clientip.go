package httpserver

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the client address for rate-limit / allowlist.
// Forwarded headers are honored only when the TCP peer is in trusted.
//
// X-Forwarded-For is walked right-to-left, skipping hops that are themselves
// in trusted. That yields the first untrusted address (the real client when
// proxies append rather than replace). Taking the leftmost hop would let a
// client forge the value under $proxy_add_x_forwarded_for.
func ClientIP(r *http.Request, trusted *TrustedProxies) string {
	peer := peerIP(r.RemoteAddr)
	peerParsed := net.ParseIP(peer)
	if trusted.empty() || peerParsed == nil || !trusted.ContainsIP(peerParsed) {
		return peer
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(hops[i])
			if ip == "" {
				continue
			}
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if trusted.ContainsIP(parsed) {
				continue
			}
			return ip
		}
		return peer
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}
	return peer
}

func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
