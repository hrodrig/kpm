package httpserver

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// ResolveTokenEnv reads token from the named environment variable.
// Empty tokenEnv means no auth configured (caller decides loopback rules).
// If tokenEnv is set, the env value must be non-empty (fail closed).
func ResolveTokenEnv(tokenEnv string) (string, error) {
	name := strings.TrimSpace(tokenEnv)
	if name == "" {
		return "", nil
	}
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("token_env %q is set but empty or unset", name)
	}
	return v, nil
}

// BearerAuth wraps next and requires Authorization: Bearer <token> when token
// is non-empty. Ops probes must not use this wrapper.
func BearerAuth(token string, next http.Handler) http.Handler {
	if strings.TrimSpace(token) == "" {
		return next
	}
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if !constantTimeEqualString(got, want) {
			http.Error(w, "unauthorized\n", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// constantTimeEqualString compares a and b in constant time when lengths match.
// Different lengths are unequal without leaking which bytes differed.
func constantTimeEqualString(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
