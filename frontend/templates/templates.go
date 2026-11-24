// Package templates holds the HTML templates and renders them. Templates are
// embedded in the binary, so the service has no files to ship alongside it.
//
// Templates receive values from the view package and nothing else. Each page
// file defines one named page; the remaining files define the partials the
// pages share (layout.html, icons.html and the search page's parts). All of
// them are parsed into a single set, so any page can use any partial.
package templates

import (
	"embed"
	"html/template"
	"io"
)

// Template names accepted by Render.
const (
	Search = "search" // the search page; data is view.Page
	Error  = "error"  // the standalone error page; data is view.ErrorPage
)

//go:embed *.html
var files embed.FS

var parsed = template.Must(template.New("").ParseFS(files, "*.html"))

// Render executes the named template with the given data.
//
// A template can fail part-way through, so callers that write HTTP responses
// should render into a buffer first and only then write the status and body.
func Render(w io.Writer, name string, data any) error {
	return parsed.ExecuteTemplate(w, name, data)
}
