package web

import (
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	appPage     = "<html>the app</html>"
	placeholder = "<html>no app</html>"
)

func withApp() fstest.MapFS {
	return fstest.MapFS{
		"app/index.html":    {Data: []byte(appPage)},
		"app/assets/x.js":   {Data: []byte("export {}")},
		"app/favicon.svg":   {Data: []byte("<svg/>")},
		"app/fonts/f.woff2": {Data: []byte("font")},
		"placeholder.html":  {Data: []byte(placeholder)},
	}
}

func withoutApp() fstest.MapFS {
	return fstest.MapFS{"placeholder.html": {Data: []byte(placeholder)}}
}

// fakeAPI answers with a status, header and body of its own.
var fakeAPI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Api", r.Method+" "+r.URL.Path)
	w.WriteHeader(http.StatusTeapot)
	_, _ = io.WriteString(w, "api says "+r.URL.Path)
})

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, nil))

	return rec
}

func TestHandlerServesTheAPIUntouched(t *testing.T) {
	h := newHandler(fakeAPI, withApp())

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/mcp"},
		{http.MethodPost, "/search"},
		{http.MethodPost, "/mcp"},
		{http.MethodDelete, "/assets/x.js"},
	} {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			rec := do(t, h, c.method, c.path)

			assert.Equal(t, http.StatusTeapot, rec.Code)
			assert.Equal(t, "api says "+c.path, rec.Body.String())
			assert.Equal(t, c.method+" "+c.path, rec.Header().Get("X-Api"))
			assert.Empty(t, rec.Header().Get("Content-Security-Policy"))
			assert.Empty(t, rec.Header().Get("Cache-Control"))
		})
	}
}

func TestHandlerServesAFile(t *testing.T) {
	h := newHandler(fakeAPI, withApp())

	for path, body := range map[string]string{"/assets/x.js": "export {}", "/favicon.svg": "<svg/>"} {
		rec := do(t, h, http.MethodGet, path)

		assert.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, body, rec.Body.String(), path)
	}

	head := do(t, h, http.MethodHead, "/assets/x.js")
	assert.Equal(t, http.StatusOK, head.Code)
	assert.Empty(t, head.Body.String())
}

func TestHandlerServesTheManifest(t *testing.T) {
	// The type would otherwise come from the host's mime.types, which may name another or none.
	before := mime.TypeByExtension(".webmanifest")

	require.NoError(t, mime.AddExtensionType(".webmanifest", "text/plain"))
	t.Cleanup(func() {
		if before != "" {
			_ = mime.AddExtensionType(".webmanifest", before)
		}
	})

	tree := withApp()
	tree["app/manifest.webmanifest"] = &fstest.MapFile{Data: []byte(`{"name": "Recall Synapsis"}`)}

	rec := do(t, newHandler(fakeAPI, tree), http.MethodGet, "/manifest.webmanifest")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/manifest+json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"name": "Recall Synapsis"}`, rec.Body.String())
}

func TestHandlerFallsBackToThePage(t *testing.T) {
	h := newHandler(fakeAPI, withApp())

	for _, path := range []string{"/", "/search", "/areas/7", "/index.html"} {
		rec := do(t, h, http.MethodGet, path)

		assert.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, appPage, rec.Body.String(), path)
		assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"), path)
	}

	for _, path := range []string{"/assets/missing.js", "/assets/"} {
		rec := do(t, h, http.MethodGet, path)

		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Body.String(), appPage, path)
	}
}

func TestHandlerSendsThePolicy(t *testing.T) {
	h := newHandler(fakeAPI, withApp())

	for _, path := range []string{"/", "/search", "/assets/x.js", "/favicon.svg", "/assets/missing.js"} {
		rec := do(t, h, http.MethodGet, path)
		csp := rec.Header().Get("Content-Security-Policy")

		assert.Contains(t, csp, "default-src 'self'", path)
		assert.Contains(t, csp, "frame-ancestors 'none'", path)
		assert.NotContains(t, csp, "unsafe-inline", path)
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"), path)
		assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"), path)
	}

	for _, path := range []string{"/assets/x.js", "/fonts/f.woff2"} {
		assert.Equal(t, "public, max-age=31536000, immutable", do(t, h, http.MethodGet, path).Header().Get("Cache-Control"), path)
	}

	for _, path := range []string{"/", "/search", "/favicon.svg"} {
		assert.Equal(t, "no-cache", do(t, h, http.MethodGet, path).Header().Get("Cache-Control"), path)
	}
}

func TestHandlerWithoutAnApp(t *testing.T) {
	h := newHandler(fakeAPI, withoutApp())

	for _, path := range []string{"/", "/search"} {
		rec := do(t, h, http.MethodGet, path)

		assert.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, placeholder, rec.Body.String(), path)
		assert.NotEmpty(t, rec.Header().Get("Content-Security-Policy"), path)
	}

	assert.Equal(t, http.StatusNotFound, do(t, h, http.MethodGet, "/assets/x.js").Code)
	assert.Equal(t, "api says /api/health", do(t, h, http.MethodGet, "/api/health").Body.String())
}

func TestHandlerServesTheEmbeddedPage(t *testing.T) {
	rec := do(t, Handler(fakeAPI), http.MethodGet, "/nowhere")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
	assert.Contains(t, rec.Body.String(), "<html")
}
