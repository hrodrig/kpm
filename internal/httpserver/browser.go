package httpserver

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// BrowserGuard rejects browser-like cross-origin / Host-mismatch requests.
// Ops paths stay exempt. Empty Origin (curl, non-browser) is allowed.
// When listen is loopback, Host must be loopback or listed in allowedHosts
// (and Origin, if set, must match the same rules) — mitigates DNS rebinding
// while supporting reverse proxies that preserve a public Host to 127.0.0.1.
func BrowserGuard(listen string, allowedHosts []string, next http.Handler) http.Handler {
	if next == nil {
		return next
	}
	skip := OpsSkip()
	loopback := IsLoopbackListen(listen)
	allow := normalizeAllowedHosts(allowedHosts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skip.matches(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if loopback {
			if !hostAllowed(r.Host, allow) {
				http.Error(w, "forbidden\n", http.StatusForbidden)
				return
			}
			if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
				if !originAllowed(origin, allow) {
					http.Error(w, "forbidden\n", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func normalizeAllowedHosts(hosts []string) map[string]bool {
	out := make(map[string]bool)
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if hp, _, err := net.SplitHostPort(h); err == nil {
			h = hp
		}
		h = strings.Trim(h, "[]")
		if h != "" {
			out[h] = true
		}
	}
	return out
}

func hostAllowed(hostport string, allow map[string]bool) bool {
	if loopbackHTTPHost(hostport) {
		return true
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	return allow[host]
}

func originAllowed(origin string, allow map[string]bool) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return hostAllowed(u.Host, allow)
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
