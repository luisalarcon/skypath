package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/luisalarcon/skypath/frontend/static"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"css/styles.css": {Data: []byte("body{margin:0}")},
		"js/app.js":      {Data: []byte("'use strict';")},
		"img/logo.svg":   {Data: []byte("<svg/>")},
	}
}

func TestAssetVersion(t *testing.T) {
	base, err := assetVersion(testAssets())
	if err != nil {
		t.Fatalf("assetVersion returned error: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(base) {
		t.Errorf("version %q is not 12 hex digits", base)
	}

	tests := []struct {
		name   string
		assets fs.FS
		same   bool
	}{
		{"identical content", testAssets(), true},
		{"changed content", func() fs.FS {
			a := testAssets()
			a["css/styles.css"] = &fstest.MapFile{Data: []byte("body{margin:1px}")}
			return a
		}(), false},
		{"renamed file", func() fs.FS {
			a := testAssets()
			a["css/main.css"] = a["css/styles.css"]
			delete(a, "css/styles.css")
			return a
		}(), false},
		{"added file", func() fs.FS {
			a := testAssets()
			a["js/extra.js"] = &fstest.MapFile{Data: []byte("1")}
			return a
		}(), false},
		{"removed file", func() fs.FS {
			a := testAssets()
			delete(a, "img/logo.svg")
			return a
		}(), false},
		{"content moved between files", fstest.MapFS{
			"css/styles.css": {Data: []byte("'use strict';")},
			"js/app.js":      {Data: []byte("body{margin:0}")},
			"img/logo.svg":   {Data: []byte("<svg/>")},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := assetVersion(tt.assets)
			if err != nil {
				t.Fatalf("assetVersion returned error: %v", err)
			}
			if (got == base) != tt.same {
				t.Errorf("version %q vs %q; same = %v, want %v", got, base, got == base, tt.same)
			}
		})
	}
}

func TestAssetVersionOfEmbeddedAssets(t *testing.T) {
	// The real assets must hash without error; walking them exercises the
	// embed.FS directory layout as well as the in-memory one above.
	got, err := assetVersion(static.FS())
	if err != nil || got == "" {
		t.Errorf("assetVersion = %q, %v", got, err)
	}
}

func notFoundMarker() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "marker: not found", http.StatusNotFound)
	})
}

func serveStatic(t *testing.T, h http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStaticHandlerCaching(t *testing.T) {
	h := staticHandler(testAssets(), "abc123", notFoundMarker())

	tests := []struct {
		name      string
		target    string
		wantCache string
	}{
		{"current version is immutable", "/static/css/styles.css?v=abc123", immutableCache},
		{"no version revalidates", "/static/css/styles.css", revalidateCache},
		{"stale version revalidates", "/static/css/styles.css?v=old", revalidateCache},
		{"empty version revalidates", "/static/css/styles.css?v=", revalidateCache},
		{"other parameters revalidate", "/static/css/styles.css?x=abc123", revalidateCache},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveStatic(t, h, http.MethodGet, tt.target, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if got := rec.Header().Get("ETag"); got != `"abc123"` {
				t.Errorf("ETag = %q, want the build version", got)
			}
			if rec.Body.String() != "body{margin:0}" {
				t.Errorf("body = %q", rec.Body.String())
			}
		})
	}
}

func TestStaticHandlerWithoutVersionNeverCachesLong(t *testing.T) {
	h := staticHandler(testAssets(), "", notFoundMarker())

	rec := serveStatic(t, h, http.MethodGet, "/static/css/styles.css?v=", nil)
	if got := rec.Header().Get("Cache-Control"); got != revalidateCache {
		t.Errorf("Cache-Control = %q, want %q", got, revalidateCache)
	}
	if rec.Header().Get("ETag") != "" {
		t.Errorf("an ETag was sent without a version")
	}
}

func TestStaticHandlerConditionalRequest(t *testing.T) {
	h := staticHandler(testAssets(), "abc123", notFoundMarker())

	rec := serveStatic(t, h, http.MethodGet, "/static/js/app.js", map[string]string{"If-None-Match": `"abc123"`})
	if rec.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("a 304 carried a body")
	}

	rec = serveStatic(t, h, http.MethodGet, "/static/js/app.js", map[string]string{"If-None-Match": `"other"`})
	if rec.Code != http.StatusOK {
		t.Errorf("status with a different ETag = %d, want 200", rec.Code)
	}
}

func TestStaticHandlerContentTypes(t *testing.T) {
	h := staticHandler(testAssets(), "v", notFoundMarker())

	tests := []struct {
		target string
		want   string
	}{
		{"/static/css/styles.css", "text/css; charset=utf-8"},
		{"/static/js/app.js", "text/javascript; charset=utf-8"},
		{"/static/img/logo.svg", "image/svg+xml"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := serveStatic(t, h, http.MethodGet, tt.target, nil)
			if got := rec.Header().Get("Content-Type"); got != tt.want {
				t.Errorf("Content-Type = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStaticHandlerHead(t *testing.T) {
	h := staticHandler(testAssets(), "v", notFoundMarker())

	rec := serveStatic(t, h, http.MethodHead, "/static/css/styles.css?v=v", nil)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("HEAD status %d with %d body bytes", rec.Code, rec.Body.Len())
	}
	if rec.Header().Get("Content-Length") != "14" || rec.Header().Get("Cache-Control") != immutableCache {
		t.Errorf("HEAD headers = %v", rec.Header())
	}
}

func TestStaticHandlerNotFound(t *testing.T) {
	h := staticHandler(testAssets(), "v", notFoundMarker())

	tests := []string{
		"/static/",
		"/static/css",
		"/static/css/",
		"/static/missing.css",
		"/static/css/missing.css",
		"/static/../css/styles.css",
		"/static/css/../css/styles.css",
		"/static//css/styles.css",
		"/static/css//styles.css",
		"/static/./css/styles.css",
		"/static/css/styles.css/",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			// httptest.NewRequest parses the target as given; the mux would
			// clean such paths, so the handler must also refuse them itself.
			rec := serveStatic(t, h, http.MethodGet, target, nil)
			if rec.Code != http.StatusNotFound || rec.Body.String() != "marker: not found\n" {
				t.Errorf("status %d body %q, want the not-found handler", rec.Code, rec.Body.String())
			}
		})
	}
}
