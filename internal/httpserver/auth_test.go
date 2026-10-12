package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hrodrig/kpm/internal/httpserver"
)

func TestResolveTokenEnvAndBearerAuth(t *testing.T) {
	tok, err := httpserver.ResolveTokenEnv("")
	if err != nil || tok != "" {
		t.Fatalf("empty env name: %q %v", tok, err)
	}
	t.Setenv("KPM_TEST_TOKEN", "")
	if _, err := httpserver.ResolveTokenEnv("KPM_TEST_TOKEN"); err == nil {
		t.Fatal("want fail closed on empty value")
	}
	t.Setenv("KPM_TEST_TOKEN", "abc")
	tok, err = httpserver.ResolveTokenEnv("KPM_TEST_TOKEN")
	if err != nil || tok != "abc" {
		t.Fatalf("got %q %v", tok, err)
	}

	h := httpserver.BearerAuth("abc", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d", rr.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth %d", rr.Code)
	}
	if httpserver.BearerAuth("", h) == nil {
		t.Fatal("empty token should pass through")
	}
}
