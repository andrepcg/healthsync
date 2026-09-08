// Package web serves the built dashboard SPA embedded into the binary.
//
// The Vite build writes to internal/web/dist. That directory is git-ignored
// (except .gitkeep) so `go build` and `go test ./...` work without Node: when
// no index.html was built, a placeholder page is served instead.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

const placeholder = `<!doctype html><html><head><meta charset="utf-8"><title>healthsync</title>
<style>body{font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem;color:#222}code{background:#eee;padding:.1rem .3rem;border-radius:3px}</style></head>
<body><h1>healthsync</h1><p>The API is running, but the dashboard UI was not built into this binary.</p>
<p>Build it with <code>make web</code> (or use the Docker image), then rebuild the binary.</p>
<p>API: <a href="/api/healthz">/api/healthz</a></p></body></html>`

// Handler serves the SPA: hashed assets with long cache lifetimes, and
// index.html for every other path so client-side routing works on reload.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	index, indexErr := fs.ReadFile(sub, "index.html")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if indexErr != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write([]byte(placeholder))
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if f, err := sub.Open(p); err == nil {
				f.Close()
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

// Built reports whether a real UI build is embedded.
func Built() bool {
	_, err := fs.ReadFile(dist, "dist/index.html")
	return err == nil
}
