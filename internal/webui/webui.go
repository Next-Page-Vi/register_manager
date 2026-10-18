// Package webui serves the embedded single-page frontend.
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Handler serves files from dist, falling back to index.html for client
// routes. Unknown /api/ paths get a JSON 404 instead of HTML.
func Handler(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if strings.HasPrefix(p, "/api/") || p == "/api" {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"code":"not_found"}`))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(p, "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(dist, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "frontend not built: run `pnpm -C web build`", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
