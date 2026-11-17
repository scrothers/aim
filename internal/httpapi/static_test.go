package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/scrothers/aim/internal/httpapi"
)

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStaticServing(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"index.html":      {Data: []byte("<html>spa</html>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
		"favicon.svg":     {Data: []byte("<svg/>")},
	}
	h := httpapi.New(httpapi.Deps{Pinger: fakePinger{}, Static: fsys})
	tests := []struct {
		name, method, path string
		status             int
		body, cache        string
	}{
		{"root", http.MethodGet, "/", http.StatusOK, "spa", "no-cache"},
		{"index", http.MethodGet, "/index.html", http.StatusOK, "spa", "no-cache"},
		{"asset", http.MethodGet, "/assets/app-1.js", http.StatusOK, "console.log", "public, max-age=31536000, immutable"},
		{"plain file", http.MethodGet, "/favicon.svg", http.StatusOK, "<svg/>", "no-cache"},
		{"spa route", http.MethodGet, "/mail/thread/x", http.StatusOK, "spa", "no-cache"},
		{"missing asset falls back", http.MethodGet, "/assets/nope.js", http.StatusOK, "spa", "no-cache"},
		{"directory falls back", http.MethodGet, "/assets", http.StatusOK, "spa", "no-cache"},
		{"traversal", http.MethodGet, "/../index.html", http.StatusOK, "spa", "no-cache"},
		{"unknown api", http.MethodGet, "/api/nope", http.StatusNotFound, "", ""},
		{"api root", http.MethodGet, "/api", http.StatusNotFound, "", ""},
		{"post elsewhere", http.MethodPost, "/mail", http.StatusNotFound, "", ""},
		{"healthz still api", http.MethodGet, "/healthz", http.StatusOK, `"status":"ok"`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := serve(h, tt.method, tt.path)
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.body) {
				t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
			}
			if tt.cache != "" && rec.Header().Get("Cache-Control") != tt.cache {
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), tt.cache)
			}
		})
	}
}

func TestStaticPlaceholder(t *testing.T) {
	t.Parallel()
	for name, fsys := range map[string]fstest.MapFS{"nil": nil, "empty": {".gitkeep": {}}} {
		deps := httpapi.Deps{Pinger: fakePinger{}}
		if fsys != nil {
			deps.Static = fsys
		}
		h := httpapi.New(deps)
		for _, p := range []string{"/", "/roster"} {
			rec := serve(h, http.MethodGet, p)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "task web:build") {
				t.Errorf("%s %s: status = %d body = %q", name, p, rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("content type = %q", ct)
			}
		}
		if rec := serve(h, http.MethodHead, "/"); rec.Body.Len() != 0 {
			t.Errorf("HEAD body = %q", rec.Body.String())
		}
	}
}
