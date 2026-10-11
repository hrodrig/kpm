package httpserver_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/httpserver"
)

func TestHealthz(t *testing.T) {
	s := httpserver.New("127.0.0.1:0", nil, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if body := rr.Body.String(); body != "ok\n" {
		t.Fatalf("body %q", body)
	}
}

func TestReadyz(t *testing.T) {
	s := httpserver.New(httpserver.DefaultListen, func(context.Context) error {
		return errors.New("store closed")
	}, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "store closed") {
		t.Fatalf("body %q", rr.Body.String())
	}

	ok := httpserver.New(httpserver.DefaultListen, func(context.Context) error { return nil }, nil)
	rr = httptest.NewRecorder()
	ok.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ready ok status %d", rr.Code)
	}
}

func TestMetrics(t *testing.T) {
	c := &httpserver.Counters{}
	c.SampleTicks.Store(2)
	c.NotifyErrors.Store(1)
	c.ObserveSampleWrite(100 * time.Millisecond)
	s := httpserver.New(httpserver.DefaultListen, nil, c)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type %q", ct)
	}
	body, _ := io.ReadAll(rr.Body)
	text := string(body)
	for _, want := range []string{
		"kpm_sample_ticks_total 2",
		"kpm_notify_errors_total 1",
		"kpm_sample_writes_total 1",
		"# TYPE kpm_sample_ticks_total counter",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestListenAndServeShutdown(t *testing.T) {
	s := httpserver.New("127.0.0.1:0", nil, nil)
	// Bind via ListenAndServe needs real port; use :0 by parsing after listen is hard.
	// Use an ephemeral port with net.Listen pattern: pass 127.0.0.1:0.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.ListenAndServe(ctx) }()

	// Give listener a moment; then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server stop")
	}
}

func TestIsLoopbackListen(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"127.0.0.1:9740", true},
		{"localhost:9740", true},
		{"[::1]:9740", true},
		{":9740", false},
		{"0.0.0.0:9740", false},
		{"192.168.1.10:9740", false},
		{"not-valid", false},
	}
	for _, tc := range cases {
		if got := httpserver.IsLoopbackListen(tc.in); got != tc.want {
			t.Fatalf("%q: got %v want %v", tc.in, got, tc.want)
		}
	}
}

func TestAccessorsAndNilObserve(t *testing.T) {
	s := httpserver.New("", nil, nil)
	if s.Addr() != httpserver.DefaultListen {
		t.Fatalf("default addr %q", s.Addr())
	}
	if s.Mux() == nil || s.Metrics() == nil {
		t.Fatal("nil mux/metrics")
	}
	var nilC *httpserver.Counters
	nilC.ObserveSampleWrite(time.Millisecond) // no panic
}
