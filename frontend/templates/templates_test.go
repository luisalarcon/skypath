package templates

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func render(t *testing.T, name string, data any) string {
	t.Helper()
	var out bytes.Buffer
	if err := Render(&out, name, data); err != nil {
		t.Fatalf("Render(%q) returned error: %v", name, err)
	}
	return out.String()
}

func assertContains(t *testing.T, html string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(html, want) {
			t.Errorf("output is missing %q", want)
		}
	}
}

func assertNotContains(t *testing.T, html string, unwanted ...string) {
	t.Helper()
	for _, s := range unwanted {
		if strings.Contains(html, s) {
			t.Errorf("output unexpectedly contains %q", s)
		}
	}
}

// tag is a start tag with its attributes, as html/template writes them.
type tag struct {
	name  string
	attrs map[string]string
	raw   string
}

var (
	tagPattern  = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9-]*)((?:\s+[^\s"'=<>/]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'>]+))?)*)\s*/?>`)
	attrPattern = regexp.MustCompile(`([^\s"'=<>/]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+)))?`)
)

// parseTags returns every start tag in the document. Attribute values are
// matched whole, so escaped text inside a value is never mistaken for an
// attribute; escaped text content never looks like a tag at all.
func parseTags(html string) []tag {
	var tags []tag
	for _, m := range tagPattern.FindAllStringSubmatch(html, -1) {
		t := tag{name: strings.ToLower(m[1]), attrs: map[string]string{}, raw: m[0]}
		for _, a := range attrPattern.FindAllStringSubmatch(m[2], -1) {
			t.attrs[strings.ToLower(a[1])] = a[2] + a[3] + a[4]
		}
		tags = append(tags, t)
	}
	return tags
}

func tagByID(t *testing.T, html, id string) tag {
	t.Helper()
	for _, tg := range parseTags(html) {
		if tg.attrs["id"] == id {
			return tg
		}
	}
	t.Fatalf("no element with id %q", id)
	return tag{}
}

// textByID returns the text directly inside the element with the given id,
// up to its first child tag.
func textByID(html, id string) string {
	m := regexp.MustCompile(`id="` + regexp.QuoteMeta(id) + `"[^>]*>([^<]*)`).FindStringSubmatch(html)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// assertCSPSafe checks the rules the content security policy enforces: no
// inline script or style, no event handler attributes, no javascript: URLs.
func assertCSPSafe(t *testing.T, html string) {
	t.Helper()
	if regexp.MustCompile(`(?is)<script\b[^>]*>\s*[^<\s]`).MatchString(html) {
		t.Errorf("output contains an inline script")
	}
	if regexp.MustCompile(`(?i)<style\b`).MatchString(html) {
		t.Errorf("output contains a style element")
	}
	for _, tg := range parseTags(html) {
		for name, value := range tg.attrs {
			switch {
			case name == "style":
				t.Errorf("inline style attribute in %s", tg.raw)
			case strings.HasPrefix(name, "on"):
				t.Errorf("event handler attribute %q in %s", name, tg.raw)
			case (name == "href" || name == "src" || name == "action" || name == "formaction") &&
				strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "javascript:"):
				t.Errorf("javascript: URL in %s", tg.raw)
			}
		}
		if tg.name == "script" && tg.attrs["src"] == "" {
			t.Errorf("script without src: %s", tg.raw)
		}
	}
}

// assertUniqueIDs fails when any id appears more than once.
func assertUniqueIDs(t *testing.T, html string) {
	t.Helper()
	seen := map[string]int{}
	for _, tg := range parseTags(html) {
		if id, ok := tg.attrs["id"]; ok {
			seen[id]++
		}
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("id %q appears %d times", id, n)
		}
	}
}

func TestRenderUnknownTemplate(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, "missing", nil); err == nil {
		t.Errorf("Render of an unknown template should fail")
	}
}

func TestErrorPages(t *testing.T) {
	for _, f := range errorFixtures() {
		t.Run(f.name, func(t *testing.T) {
			html := render(t, Error, f.page)
			assertContains(t, html,
				"<!doctype html>",
				"<title>"+f.page.Title+"</title>",
				"/static/css/styles.css?v="+testAssetVersion,
				"<h1 class=\"error-page__heading\">"+f.page.Heading+"</h1>",
				f.page.Message,
				`href="/"`,
				`<main id="main"`,
				`class="skip-link" href="#main"`,
			)
			if n := strings.Count(html, "<h1"); n != 1 {
				t.Errorf("error page has %d h1 elements, want 1", n)
			}
			assertCSPSafe(t, html)
			assertUniqueIDs(t, html)
		})
	}
}

func TestErrorPageEscapes(t *testing.T) {
	page := notFoundPage()
	page.Title, page.Heading, page.Message = hostile, hostile, hostile
	html := render(t, Error, page)
	assertNotContains(t, html, "<script>alert(1)", "<img src=x")
	assertCSPSafe(t, html)
}

func TestSearchPageLayout(t *testing.T) {
	for _, f := range pageFixtures() {
		t.Run(f.name, func(t *testing.T) {
			html := render(t, Search, f.page)
			assertContains(t, html,
				"<!doctype html>",
				`<html lang="en">`,
				"/static/css/styles.css?v="+testAssetVersion,
				`<script src="/static/js/app.js?v=`+testAssetVersion+`" defer></script>`,
				`class="skip-link" href="#main"`,
				`<header class="site-header">`,
				`<main id="main"`,
				`<footer class="site-footer">`,
			)
			if n := strings.Count(html, "<h1"); n != 1 {
				t.Errorf("page has %d h1 elements, want 1", n)
			}
			assertCSPSafe(t, html)
			assertUniqueIDs(t, html)
		})
	}
}
