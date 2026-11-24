package templates

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/luisalarcon/skypath/frontend/static"
)

// TestWritePreview renders every fixture to a static site so the interface
// can be reviewed in a browser without the API or the handlers. It only runs
// when SKYPATH_PREVIEW_DIR names an output directory:
//
//	SKYPATH_PREVIEW_DIR=/tmp/skypath-preview go test ./templates -run TestWritePreview
//
// The directory receives one HTML file per state (initial.html,
// results.html, ...) and the embedded assets under static/, matching the
// /static/ paths the pages reference. Serve it from its root with any static
// file server, for example:
//
//	python -m http.server --directory /tmp/skypath-preview 8080
func TestWritePreview(t *testing.T) {
	dir := os.Getenv("SKYPATH_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SKYPATH_PREVIEW_DIR to write the preview site")
	}

	write := func(name string, tmpl string, data any) {
		f, err := os.Create(filepath.Join(dir, name+".html"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := Render(f, tmpl, data); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range pageFixtures() {
		write(f.name, Search, f.page)
	}
	for _, f := range errorFixtures() {
		write(f.name, Error, f.page)
	}

	err := fs.WalkDir(static.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, "static", filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(static.FS(), path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("preview written to %s", dir)
}
