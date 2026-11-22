package web

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

const policy = "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'"

// Handler passes /api/, /mcp and every method but GET and HEAD to api. Otherwise it serves a built file
// when the path names one and the page for every other path outside /assets/, with the headers a page
// needs. With no app built in, it serves one page that says so.
func Handler(api http.Handler) http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embedded tree always has dist
	}

	return newHandler(api, root)
}

// newHandler takes the tree Handler embeds: app/ when a web build ran, and placeholder.html.
func newHandler(api http.Handler, root fs.FS) http.Handler {
	var files fs.FS

	page, err := fs.ReadFile(root, "app/index.html")
	if err != nil {
		page, err = fs.ReadFile(root, "placeholder.html")
		if err != nil {
			panic(err) // the embedded tree always has the placeholder
		}
	} else if files, err = fs.Sub(root, "app"); err != nil {
		panic(err) // app/index.html was just read
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) ||
			r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)

			return
		}

		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-cache")

		name := strings.TrimPrefix(r.URL.Path, "/")
		if files != nil && name != "" && name != "index.html" {
			if info, err := fs.Stat(files, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(r.URL.Path, "/assets/") || strings.HasPrefix(r.URL.Path, "/fonts/") {
					h.Set("Cache-Control", "public, max-age=31536000, immutable")
				}

				// Go's own table lacks this extension, and the host's mime.types may too.
				if name == "manifest.webmanifest" {
					h.Set("Content-Type", "application/manifest+json")
				}

				http.ServeFileFS(w, r, files, name) //nolint:gosec // name is a file the app tree just stat-ed

				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)

			return
		}

		h.Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(page))
	})
}
