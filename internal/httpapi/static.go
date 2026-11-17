package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	cacheImmutable = "public, max-age=31536000, immutable"
	cacheNone      = "no-cache"
	indexName      = "index.html"
)

const placeholderHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>AIM</title>
<style>body{font:16px/1.5 system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;padding:0 16px;background:#fafafa;color:#222}
@media(prefers-color-scheme:dark){body{background:#161616;color:#ddd}}code{padding:2px 6px;border-radius:4px;background:rgba(127,127,127,.2)}</style>
</head><body><p>AIM web UI not built &mdash; run <code>task web:build</code></p></body></html>
`

// mountStatic serves the web UI for every request no API route matched.
func mountStatic(router chi.Router, fsys fs.FS) {
	router.NotFound(staticHandler(fsys))
}

func staticHandler(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || p == "/api" || strings.HasPrefix(p, "/api/") {
			http.NotFound(w, r)
			return
		}
		if fsys == nil {
			servePlaceholder(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+p), "/")
		if name != "" && name != indexName {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				cache := cacheNone
				if strings.HasPrefix(name, "assets/") {
					cache = cacheImmutable
				}
				serveFile(w, r, fsys, name, cache)
				return
			}
		}
		if _, err := fs.Stat(fsys, indexName); err != nil {
			servePlaceholder(w, r)
			return
		}
		serveFile(w, r, fsys, indexName, cacheNone) // SPA fallback
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name, cache string) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.Error(w, "read failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

func servePlaceholder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", cacheNone)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(placeholderHTML))
	}
}
