package httpserver

import (
	"net"
	"strings"
)

// IsLoopbackListen reports whether host in "host:port" (or "[host]:port") is
// a loopback address. Empty host (":port") is treated as non-loopback
// (binds all interfaces).
func IsLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Try wrapping bare IPv6 without brackets failure paths.
		if h, _, e2 := net.SplitHostPort("[" + addr + "]"); e2 == nil {
			host = h
		} else {
			return false
		}
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
