package httpserver

import (
	"path"
	"strings"
)

// PathSkip lists request paths exempt from rate limiting or IP allowlist.
type PathSkip struct {
	Exact    []string
	Prefixes []string
}

// OpsSkip returns paths that must stay reachable without rate limits or
// allowlist checks (liveness, readiness, Prometheus scrape, embedded UI static).
func OpsSkip() PathSkip {
	return PathSkip{
		Exact:    []string{"/livez", "/healthz", "/readyz", "/metrics"},
		Prefixes: []string{"/static/"},
	}
}

func (s PathSkip) matches(rawPath string) bool {
	if rawPath == "" {
		return false
	}
	cleaned := path.Clean(rawPath)
	for _, p := range s.Exact {
		if cleaned == p {
			return true
		}
	}
	for _, p := range s.Prefixes {
		if strings.HasPrefix(cleaned, p) {
			return true
		}
		if strings.HasSuffix(p, "/") && cleaned == strings.TrimSuffix(p, "/") {
			return true
		}
	}
	return false
}
