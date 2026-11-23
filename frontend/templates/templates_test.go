package templates

import (
	"bytes"
	"strings"
	"testing"

	"github.com/luisalarcon/skypath/frontend/internal/view"
)

func TestRenderSearchPage(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, Search, view.Page{Title: "SkyPath", AssetVersion: "test"}); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	html := out.String()
	for _, want := range []string{"<!doctype html>", "<title>SkyPath</title>", "styles.css?v=test"} {
		if !strings.Contains(html, want) {
			t.Errorf("search page is missing %q", want)
		}
	}
}

func TestRenderErrorPage(t *testing.T) {
	var out bytes.Buffer
	page := view.ErrorPage{Title: "Not found", Status: 404, Heading: "Page not found", Message: "Nothing lives at this address."}
	if err := Render(&out, Error, page); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if html := out.String(); !strings.Contains(html, "Page not found") {
		t.Errorf("error page is missing its heading")
	}
}

func TestRenderEscapesUserInput(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, Search, view.Page{Title: "<script>alert(1)</script>"}); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if strings.Contains(out.String(), "<script>alert(1)</script>") {
		t.Errorf("template output contains unescaped input")
	}
}

func TestRenderUnknownTemplate(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, "missing", nil); err == nil {
		t.Errorf("Render of an unknown template should fail")
	}
}
