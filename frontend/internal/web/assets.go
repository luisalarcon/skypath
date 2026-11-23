package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	staticPrefix = "/static/"

	// immutableCache lets browsers keep a versioned asset for a year without
	// asking again. That is safe because a new build changes the version and
	// with it the URL.
	immutableCache = "public, max-age=31536000, immutable"
	// revalidateCache lets browsers store an asset but makes them check it,
	// which the ETag turns into a cheap 304.
	revalidateCache = "no-cache"
)

// assetVersion returns a short digest of the names and contents of every
// file in fsys. It is computed once at startup and appended to asset URLs, so
// a build with different assets gets different URLs and no stale copy is
// ever served from a browser cache.
func assetVersion(fsys fs.FS) (string, error) {
	sum := sha256.New()
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		// WalkDir visits files in lexical order, so the digest is stable. The
		// separators keep ("ab", "c") distinct from ("a", "bc").
		sum.Write([]byte(name))
		sum.Write([]byte{0})
		sum.Write(content)
		sum.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil))[:12], nil
}

// staticHandler serves the files of fsys under /static/.
//
// A request whose "v" parameter equals version is for exactly this build's
// asset and is cached for as long as browsers allow. Any other request, such
// as an unversioned link or a stale page asking for last week's version,
// still gets the current file but must revalidate, so an old URL can never
// pin new content in a cache. An empty version disables long-lived caching.
//
// Requests for anything that is not a file are answered by notFound, which
// also covers directories, so the asset tree is never listed.
func staticHandler(fsys fs.FS, version string, notFound http.Handler) http.Handler {
	etag := ""
	if version != "" {
		etag = `"` + version + `"`
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, staticPrefix)
		if !fs.ValidPath(name) || name == "." {
			notFound.ServeHTTP(w, r)
			return
		}
		info, err := fs.Stat(fsys, name)
		if err != nil || info.IsDir() {
			notFound.ServeHTTP(w, r)
			return
		}
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			notFound.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		h.Set("Content-Type", contentType(name))
		if etag != "" {
			h.Set("ETag", etag)
		}
		if version != "" && r.URL.Query().Get("v") == version {
			h.Set("Cache-Control", immutableCache)
		} else {
			h.Set("Cache-Control", revalidateCache)
		}
		// ServeContent adds conditional requests, ranges and HEAD handling.
		// The embedded files carry no modification time, so none is sent.
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
	})
}

// contentType names the media type of an asset. The two types the page
// depends on are fixed here because the system's MIME database, which the
// standard library consults, can map them differently on some machines, and a
// stylesheet served as the wrong type is ignored by browsers.
func contentType(name string) string {
	switch path.Ext(name) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	}
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}
