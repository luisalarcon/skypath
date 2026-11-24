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

func TestSearchFormFields(t *testing.T) {
	html := render(t, Search, resultsPage())

	form := tagByID(t, html, "search-form")
	if form.attrs["method"] != "get" || form.attrs["action"] != "/" {
		t.Errorf("form must GET /, got method=%q action=%q", form.attrs["method"], form.attrs["action"])
	}
	for _, f := range []struct{ id, label, value string }{
		{"origin", "From", "JFK"},
		{"destination", "To", "LAX"},
		{"date", "Date", "2024-03-15"},
	} {
		assertContains(t, html, `<label class="field__label" for="`+f.id+`">`+f.label+`</label>`)
		input := tagByID(t, html, f.id)
		if input.attrs["name"] != f.id {
			t.Errorf("input %s has name %q", f.id, input.attrs["name"])
		}
		if input.attrs["value"] != f.value {
			t.Errorf("input %s has value %q, want %q", f.id, input.attrs["value"], f.value)
		}
		if got, want := input.attrs["aria-describedby"], f.id+"-error "+f.id+"-hint"; got != want {
			t.Errorf("input %s aria-describedby = %q, want %q", f.id, got, want)
		}
		if _, ok := input.attrs["aria-invalid"]; ok {
			t.Errorf("valid input %s is marked aria-invalid", f.id)
		}
		if msg := textByID(html, f.id+"-error"); msg != "" {
			t.Errorf("valid input %s shows message %q", f.id, msg)
		}
	}

	for _, id := range []string{"origin", "destination"} {
		if got := tagByID(t, html, id).attrs["list"]; got != "airports" {
			t.Errorf("input %s list = %q, want airports", id, got)
		}
	}
	assertContains(t, html,
		`<datalist id="airports">`,
		`<option value="JFK">New York · John F. Kennedy International</option>`,
		`<option value="NRT">Tokyo · Narita International</option>`,
	)
	if got := textByID(html, "origin-hint"); got != "New York · John F. Kennedy International" {
		t.Errorf("origin hint = %q", got)
	}
	if got := textByID(html, "destination-hint"); got != "Los Angeles · Los Angeles International" {
		t.Errorf("destination hint = %q", got)
	}

	date := tagByID(t, html, "date")
	if date.attrs["type"] != "date" || date.attrs["min"] != "2024-03-15" || date.attrs["max"] != "2024-03-16" {
		t.Errorf("date input type/min/max = %q/%q/%q", date.attrs["type"], date.attrs["min"], date.attrs["max"])
	}
	assertContains(t, html, "Mar 15, 2024 – Mar 16, 2024")

	// The swap control needs script, so it starts hidden.
	assertContains(t, html, `<div class="search-form__swap" hidden>`)
}

func TestSearchFormWithoutBounds(t *testing.T) {
	html := render(t, Search, unavailablePage())
	date := tagByID(t, html, "date")
	for _, attr := range []string{"min", "max"} {
		if _, ok := date.attrs[attr]; ok {
			t.Errorf("date input has %s although the bounds are unknown", attr)
		}
	}
	if strings.Contains(html, "<option") {
		t.Errorf("datalist has options although there are no airports")
	}
}

func TestFieldErrorsAreWired(t *testing.T) {
	page := invalidPage()
	html := render(t, Search, page)
	for id, want := range map[string]string{
		"origin":      page.Form.Errors.Origin,
		"destination": page.Form.Errors.Destination,
		"date":        page.Form.Errors.Date,
	} {
		input := tagByID(t, html, id)
		if input.attrs["aria-invalid"] != "true" {
			t.Errorf("input %s is not marked aria-invalid", id)
		}
		if !strings.Contains(" "+input.attrs["aria-describedby"]+" ", " "+id+"-error ") {
			t.Errorf("input %s is not described by its message: %q", id, input.attrs["aria-describedby"])
		}
		if got := textByID(html, id+"-error"); got != want {
			t.Errorf("message for %s = %q, want %q", id, got, want)
		}
	}
	// The form keeps what the user typed.
	if got := tagByID(t, html, "origin").attrs["value"]; got != "XXX" {
		t.Errorf("origin value = %q, want XXX", got)
	}
}

func TestResultsState(t *testing.T) {
	html := render(t, Search, resultsPage())
	assertContains(t, html,
		"<title>JFK to LAX · Mar 15, 2024 · SkyPath</title>",
		`id="results-heading" tabindex="-1"`,
		`New York <span class="results-head__code">(JFK)</span>`,
		`Los Angeles <span class="results-head__code">(LAX)</span>`,
		`<time datetime="2024-03-15">Friday, March 15, 2024</time>`,
		`<dt>Itineraries</dt><dd>2</dd>`,
		`<dt>Direct</dt><dd>1</dd>`,
		`<dt>One stop</dt><dd>1</dd>`,
		"Times are local to each airport.",
		// Summary rows.
		`<time class="route__time" datetime="2024-03-15T08:30:00-04:00">8:30 AM</time>`,
		`<p class="card__price">$299.00</p>`,
		`<p class="route__stops">1 stop · ORD</p>`,
		`<li class="tag">Fastest</li>`,
		`<li class="tag">Cheapest</li>`,
		// Accessible names summarise each card.
		`aria-label="8:30 AM JFK to 11:45 AM LAX, 6h 15m, Direct, $299.00, Fastest, Cheapest"`,
		`aria-label="7:00 AM JFK to 11:30 AM LAX, 7h 30m, 1 stop via ORD, $414.00"`,
		// Timeline.
		"SP110 · SkyPath Airways · A319",
		"SP120 · SkyPath Airways · A320",
		`<time datetime="2024-03-15T08:30:00-05:00">8:30 AM</time>`,
		"O&#39;Hare International · UTC-5",
		`<span class="layover__duration">45m layover</span> <span class="layover__where">in Chicago (ORD)</span>`,
		`<span class="layover__kind">Domestic connection</span>`,
	)
	assertNotContains(t, html, "Two stops", "day-offset", "No itineraries found")

	if n := strings.Count(html, `<li class="itinerary"`); n != 2 {
		t.Errorf("rendered %d itinerary cards, want 2", n)
	}
	var cards []tag
	for _, tg := range parseTags(html) {
		if tg.attrs["class"] == "itinerary" {
			cards = append(cards, tg)
		}
	}
	want := [][3]string{{"375", "29900", "0"}, {"450", "41400", "1"}}
	for i, c := range cards {
		got := [3]string{c.attrs["data-duration-minutes"], c.attrs["data-price-cents"], c.attrs["data-stops"]}
		if got != want[i] {
			t.Errorf("card %d data attributes = %v, want %v", i, got, want[i])
		}
	}
	if got := tagByID(t, html, "results-heading").name; got != "h2" {
		t.Errorf("results heading is <%s>, want <h2>", got)
	}
}

func TestTwoStopResults(t *testing.T) {
	html := render(t, Search, twoStopPage())
	assertContains(t, html,
		`<dt>Two stops</dt><dd>1</dd>`,
		`<p class="route__stops">2 stops · DEN, LAX</p>`,
		`<span class="route__code">NRT <span class="day-offset">&#43;2 days</span></span>`,
		`<time datetime="2024-03-17T03:00:00&#43;09:00">3:00 AM</time> <span class="day-offset">&#43;2 days</span>`,
		`<li class="layover layover--international">`,
		`<span class="layover__kind">International connection</span>`,
		`<span class="layover__duration">2h 45m layover</span> <span class="layover__where">in Los Angeles (LAX)</span>`,
		"SP501 · SkyPath Airways · B777",
		`aria-label="2:00 PM SFO to 3:00 AM NRT &#43;2 days, 21h, 2 stops via DEN, LAX, $1,137.00, Fastest, Cheapest"`,
	)
	assertNotContains(t, html, "<dt>Direct</dt>", "<dt>One stop</dt>")
	if n := strings.Count(html, `<span class="route__stop">`); n != 2 {
		t.Errorf("route shows %d stop dots, want 2", n)
	}
	if n := strings.Count(html, `<li class="leg">`); n != 3 {
		t.Errorf("timeline shows %d flights, want 3", n)
	}
}

func TestInitialState(t *testing.T) {
	html := render(t, Search, initialPage())
	assertContains(t, html,
		"<title>SkyPath · Flight search</title>",
		`<section class="intro"`,
		"The schedule covers Mar 15, 2024 to Mar 16, 2024.",
	)
	assertNotContains(t, html, `id="results-heading"`, `class="notice `, `class="itinerary"`, `aria-invalid`)
}

func TestEmptyState(t *testing.T) {
	html := render(t, Search, emptyPage())
	assertContains(t, html,
		`id="results-heading" tabindex="-1">No itineraries found</h2>`,
		"Nothing connects New York (JFK) to Los Angeles (LAX)",
		`<time datetime="2024-03-20">Wednesday, March 20, 2024</time>`,
		"The schedule covers Mar 15, 2024 to Mar 16, 2024.",
		`href="/?origin=JFK&amp;destination=LAX&amp;date=2024-03-15"`,
		"Search Mar 15, 2024 instead",
	)
	assertNotContains(t, html, `class="itinerary"`, `class="tally"`, `<section class="intro"`)
}

func TestEmptyStateWithoutCoverage(t *testing.T) {
	page := emptyPage()
	page.Dataset = nil
	html := render(t, Search, page)
	assertContains(t, html, "No itineraries found", "Nothing connects New York (JFK)")
	assertNotContains(t, html, "The schedule covers", "data-search-link")
}

func TestInvalidState(t *testing.T) {
	html := render(t, Search, invalidPage())
	assertContains(t, html, "Unknown airport code XXX.", "Destination is required.")
	// Messages sit beside the fields; there is no results content at all.
	if got := strings.TrimSpace(textByID(html, "results")); got != "" {
		t.Errorf("results area is not empty: %q", got)
	}
	assertNotContains(t, html, `id="results-heading"`, `<section class="intro"`, `class="notice `)
}

func TestUnavailableState(t *testing.T) {
	page := unavailablePage()
	html := render(t, Search, page)
	assertContains(t, html,
		`<div class="notice notice--error">`,
		`<h2 class="notice__title">Search is temporarily unavailable</h2>`,
		"Your search is still filled in",
		`type="submit" form="search-form">Try again</button>`,
	)
	// The form keeps its values.
	if got := tagByID(t, html, "origin").attrs["value"]; got != "JFK" {
		t.Errorf("origin value = %q, want JFK", got)
	}
	if got := tagByID(t, html, "destination").attrs["value"]; got != "LAX" {
		t.Errorf("destination value = %q, want LAX", got)
	}
	assertNotContains(t, html, `id="results-heading"`, `<section class="intro"`, `class="itinerary"`)
}

// Regions the script swaps from a fetched page must each exist exactly once
// in every state, even when empty.
func TestSwapRegions(t *testing.T) {
	regions := []string{"notice", "results", "origin-error", "destination-error", "date-error", "origin-hint", "destination-hint", "date-hint"}
	for _, f := range pageFixtures() {
		t.Run(f.name, func(t *testing.T) {
			html := render(t, Search, f.page)
			for _, id := range regions {
				if n := strings.Count(html, ` id="`+id+`"`); n != 1 {
					t.Errorf("region %q appears %d times, want 1", id, n)
				}
			}
		})
	}
}

func TestEscapesHostileInput(t *testing.T) {
	for name, page := range map[string]any{"results": hostilePage(), "empty": hostileEmptyPage()} {
		t.Run(name, func(t *testing.T) {
			html := render(t, Search, page)
			assertNotContains(t, html, "<script>alert(1)", "<img src=x", `"><script>`)
			assertCSPSafe(t, html)
			assertUniqueIDs(t, html)
			for _, tg := range parseTags(html) {
				if tg.name == "img" {
					t.Errorf("hostile input produced an element: %s", tg.raw)
				}
			}
		})
	}
	// The value attribute keeps the text intact once unescaped by the browser.
	html := render(t, Search, hostilePage())
	if got := tagByID(t, html, "origin").attrs["value"]; !strings.Contains(got, "&lt;script&gt;") && !strings.Contains(got, "&#34;&gt;&lt;script") {
		t.Errorf("origin value is not escaped as text: %q", got)
	}
}

func TestEnhancementHooks(t *testing.T) {
	html := render(t, Search, initialPage())
	assertContains(t, html,
		`<div class="visually-hidden" id="announcer" role="status" aria-live="polite" aria-atomic="true"></div>`,
		`<template id="results-skeleton">`,
		`data-loading-label="Searching…"`,
		`<div class="notices" id="notice" tabindex="-1">`,
	)
	if n := strings.Count(html, `class="skeleton-card"`); n != 3 {
		t.Errorf("skeleton has %d cards, want 3", n)
	}
	// Skeleton markup lives only inside the inert template element.
	start, end := strings.Index(html, "<template"), strings.Index(html, "</template>")
	if first := strings.Index(html, "skeleton-card"); first < start || first > end {
		t.Errorf("skeleton markup found outside its template")
	}
}
