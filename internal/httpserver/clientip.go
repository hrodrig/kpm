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
	if ip, hasXFF := clientIPFromXFF(r.Header.Values("X-Forwarded-For"), trusted); hasXFF {
		if ip != "" {
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

// clientIPFromXFF walks all X-Forwarded-For header lines right-to-left so a
// client-forged first header cannot shadow a header appended by a reverse proxy.
func clientIPFromXFF(values []string, trusted *TrustedProxies) (ip string, hasNonEmpty bool) {
	for vi := len(values) - 1; vi >= 0; vi-- {
		raw := strings.TrimSpace(values[vi])
		if raw == "" {
			continue
		}
		hasNonEmpty = true
		hops := strings.Split(raw, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			hop := strings.TrimSpace(hops[i])
			if hop == "" {
				continue
			}
			parsed := net.ParseIP(hop)
			if parsed == nil || trusted.ContainsIP(parsed) {
				continue
			}
			return hop, true
		}
	}
	return "", hasNonEmpty
}

func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
