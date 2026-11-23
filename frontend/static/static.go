// Package static embeds the stylesheet, script and icon served under
// /static/. Nothing is loaded from a CDN, so the application works offline.
package static

import (
	"embed"
	"io/fs"
)

//go:embed css js img
var files embed.FS

// FS returns the embedded asset tree, rooted so that "css/styles.css",
// "js/app.js" and "img/icon.svg" resolve directly.
func FS() fs.FS {
	return files
}
