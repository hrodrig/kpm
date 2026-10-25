// Package httpserver provides the daemon HTTP mux for ops probes and (later) MCP.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// DefaultListen is the loopback bind used when mcp.listen is empty.
const DefaultListen = "127.0.0.1:9740"

// ReadyFunc reports whether the process is ready to serve traffic.
// Return nil for ready; non-nil marks /readyz as 503.
type ReadyFunc func(ctx context.Context) error

// Counters holds process metrics for GET /metrics (Prometheus text).
type Counters struct {
	SampleTicks      atomic.Int64
	SampleErrors     atomic.Int64 // transient RunOnce failures while retrying
	NotifyErrors     atomic.Int64
	SampleWriteNanos atomic.Int64 // cumulative nanoseconds spent inserting samples
	SampleWriteCount atomic.Int64
}

// Server is a small net/http mux with ops routes. Extra handlers may be
// registered on Mux() before ListenAndServe (MCP in a later slice).
type Server struct {
	addr    string
	ready   ReadyFunc
	metrics *Counters
	mux     *http.ServeMux
	handler http.Handler
	access  AccessControl
}

// New builds a Server. ready may be nil (always ready). metrics may be nil
// (zeros in /metrics).
func New(addr string, ready ReadyFunc, metrics *Counters) *Server {
	if addr == "" {
		addr = DefaultListen
	}
	if metrics == nil {
		metrics = &Counters{}
	}
	s := &Server{addr: addr, ready: ready, metrics: metrics, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /livez", s.handleHealthz)
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.handler = s.mux
	return s
}

// Addr returns the configured listen address.
func (s *Server) Addr() string { return s.addr }

// Mux returns the underlying mux for mounting additional routes.
func (s *Server) Mux() *http.ServeMux { return s.mux }

// Metrics returns the process counters (never nil after New).
func (s *Server) Metrics() *Counters { return s.metrics }

// SetAccessControl installs allowlist / rate-limit middleware (gghstats mold).
// Call before ListenAndServe. Ops paths remain exempt.
func (s *Server) SetAccessControl(ac AccessControl) {
	s.access = ac
	s.handler = ac.Wrap(s.mux)
}

// Access returns the installed access control (may be zero).
func (s *Server) Access() AccessControl { return s.access }

// Handler returns the HTTP handler (for httptest), including access middleware.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if s.ready != nil {
		if err := s.ready(r.Context()); err != nil {
			http.Error(w, err.Error()+"\n", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	m := s.metrics
	ticks := m.SampleTicks.Load()
	sampleErrs := m.SampleErrors.Load()
	notifyErrs := m.NotifyErrors.Load()
	writeN := m.SampleWriteCount.Load()
	writeNanos := m.SampleWriteNanos.Load()
	var avgSec float64
	if writeN > 0 {
		avgSec = float64(writeNanos) / float64(writeN) / float64(time.Second)
	}
	_, _ = fmt.Fprintf(w, `# HELP kpm_sample_ticks_total Sample daemon ticks completed.
# TYPE kpm_sample_ticks_total counter
kpm_sample_ticks_total %d
# HELP kpm_sample_errors_total Transient sample tick failures (retried).
# TYPE kpm_sample_errors_total counter
kpm_sample_errors_total %d
# HELP kpm_notify_errors_total Notify fan-out evaluation errors.
# TYPE kpm_notify_errors_total counter
kpm_notify_errors_total %d
# HELP kpm_sample_write_seconds_avg Average SQLite sample insert duration in seconds.
# TYPE kpm_sample_write_seconds_avg gauge
kpm_sample_write_seconds_avg %g
# HELP kpm_sample_writes_total Sample batch inserts into SQLite.
# TYPE kpm_sample_writes_total counter
kpm_sample_writes_total %d
`, ticks, sampleErrs, notifyErrs, avgSec, writeN)
}

// ListenAndServe binds addr and serves until ctx is cancelled.
// It returns nil on graceful shutdown; other errors are returned.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("httpserver listen %s: %w", s.addr, err)
	}
	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- err
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		if s.access.RateLimit != nil {
			s.access.RateLimit.Shutdown()
		}
		return <-errCh
	case err := <-errCh:
		if s.access.RateLimit != nil {
			s.access.RateLimit.Shutdown()
		}
		return err
	}
}

// ObserveSampleWrite records one SQLite insert batch duration.
func (c *Counters) ObserveSampleWrite(d time.Duration) {
	if c == nil {
		return
	}
	c.SampleWriteCount.Add(1)
	c.SampleWriteNanos.Add(d.Nanoseconds())
}
