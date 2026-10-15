package httpserver

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// BrowserGuard rejects browser-like cross-origin / Host-mismatch requests.
// Ops paths stay exempt. Empty Origin (curl, non-browser) is allowed.
// When listen is loopback, Host must be a loopback name and Origin (if set)
// must also be loopback — mitigates DNS rebinding against tokenless loopback.
func BrowserGuard(listen string, next http.Handler) http.Handler {
	if next == nil {
		return next
	}
	skip := OpsSkip()
	loopback := IsLoopbackListen(listen)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skip.matches(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if loopback {
			if !loopbackHTTPHost(r.Host) {
				http.Error(w, "forbidden\n", http.StatusForbidden)
				return
			}
			if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
				if !loopbackHTTPOrigin(origin) {
					http.Error(w, "forbidden\n", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHTTPHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loopbackHTTPOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return loopbackHTTPHost(u.Host)
}
