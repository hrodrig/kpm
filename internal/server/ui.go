package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed ui/index.html ui/static/*
var uiFS embed.FS

// uiCSP locks the embedded analyze-local UI to same-origin scripts/styles/API
// with no inline script execution and no framing.
const uiCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

func setUISecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", uiCSP)
}

func withUISecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setUISecurityHeaders(w)
		next.ServeHTTP(w, r)
	})
}

// MountUI registers the analyze-local charts page and static assets.
// UI itself is not bearer-wrapped (browser address bar); JS calls /api/v1
// with Authorization from sessionStorage after unlock on 401.
func MountUI(mux *http.ServeMux) {
	index := withUISecurityHeaders(http.HandlerFunc(serveIndex))
	mux.Handle("GET /{$}", index)
	mux.Handle("GET /ui", index)
	mux.Handle("GET /ui/", index)

	static, err := fs.Sub(uiFS, "ui/static")
	if err != nil {
		panic("server: embed ui/static: " + err.Error())
	}
	mux.Handle("GET /static/", withUISecurityHeaders(http.StripPrefix("/static/", http.FileServer(http.FS(static)))))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	b, err := uiFS.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "ui missing\n", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// MountAll registers JSON API + charts UI when server mode is on.
func MountAll(mux *http.ServeMux, d *Deps, bearerToken string) {
	Mount(mux, d, bearerToken)
	MountUI(mux)
}

// UIPaths reports whether path is a charts UI route (for docs/tests).
func UIPaths(path string) bool {
	return path == "/" || path == "/ui" || path == "/ui/" || strings.HasPrefix(path, "/static/")
}
