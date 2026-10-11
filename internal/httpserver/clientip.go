package httpserver

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the client address for rate-limit / allowlist.
// Forwarded headers are honored only when the TCP peer is in trusted.
func ClientIP(r *http.Request, trusted *TrustedProxies) string {
	peer := peerIP(r.RemoteAddr)
	peerParsed := net.ParseIP(peer)
	if trusted.empty() || peerParsed == nil || !trusted.ContainsIP(peerParsed) {
		return peer
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		for _, hop := range strings.Split(xff, ",") {
			ip := strings.TrimSpace(hop)
			if ip == "" {
				continue
			}
			if net.ParseIP(ip) != nil {
				return ip
			}
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
