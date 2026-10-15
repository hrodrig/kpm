package httpserver

import "strings"

// PathSkip lists request paths exempt from rate limiting or IP allowlist.
type PathSkip struct {
	Exact    []string
	Prefixes []string
}

// OpsSkip returns paths that must stay reachable without rate limits or
// allowlist checks (liveness, readiness, Prometheus scrape, embedded UI static).
func OpsSkip() PathSkip {
	return PathSkip{
		Exact:    []string{"/healthz", "/readyz", "/metrics"},
		Prefixes: []string{"/static/"},
	}
}

func (s PathSkip) matches(path string) bool {
	for _, p := range s.Exact {
		if path == p {
			return true
		}
	}
	for _, p := range s.Prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
