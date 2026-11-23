// Package static embeds the stylesheet and script served under /static/.
// Nothing is loaded from a CDN, so the application works offline.
package static

import (
	"embed"
	"io/fs"
)

//go:embed css js
var files embed.FS

// FS returns the embedded asset tree, rooted so that "css/styles.css" and
// "js/app.js" resolve directly.
func FS() fs.FS {
	return files
}
