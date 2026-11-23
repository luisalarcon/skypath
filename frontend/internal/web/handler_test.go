package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/view"
	"github.com/luisalarcon/skypath/frontend/static"
	"github.com/luisalarcon/skypath/frontend/templates"
)

// renderCall is one template execution seen by the recording renderer.
type renderCall struct {
	Name string
	Data any
}

// renderRecorder stands in for the template renderer. The real templates arrive
// from another branch, so the tests assert on what the handler hands to the
// renderer rather than on HTML.
type renderRecorder struct {
	mu    sync.Mutex
	calls []renderCall
	// fail, when set, decides per template name whether rendering fails.
	fail func(name string) error
	// partial is written before a failure, to prove it never reaches the client.
	partial string
}

func (r *renderRecorder) render(w io.Writer, name string, data any) error {
	r.mu.Lock()
	r.calls = append(r.calls, renderCall{name, data})
	r.mu.Unlock()
	if r.fail != nil {
		if err := r.fail(name); err != nil {
			_, _ = io.WriteString(w, r.partial)
			return err
		}
	}
	_, err := fmt.Fprintf(w, "<rendered %s>", name)
	return err
}

func (r *renderRecorder) last(t *testing.T) renderCall {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		t.Fatal("nothing was rendered")
	}
	return r.calls[len(r.calls)-1]
}

func (r *renderRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// testRig bundles a handler with the fakes behind it.
type testRig struct {
	h      http.Handler
	api    *fakeAPI
	render *renderRecorder
	logs   *bytes.Buffer
}

func newRig(t *testing.T, api *fakeAPI) *testRig {
	t.Helper()
	rig := &testRig{api: api, render: &renderRecorder{}, logs: &bytes.Buffer{}}
	rig.h = NewHandler(Dependencies{
		API:    api,
		Logger: newJSONLogger(rig.logs, "debug"),
		Render: rig.render.render,
	})
	return rig
}

func (rig *testRig) do(method, target string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	rig.h.ServeHTTP(rec, req)
	return rec
}

func (rig *testRig) get(target string) *httptest.ResponseRecorder {
	return rig.do(http.MethodGet, target)
}

const searchQuery = "/?origin=JFK&destination=LAX&date=2024-03-15"

func TestStatusTable(t *testing.T) {
	timeout := fmt.Errorf("%w: %w: deadline exceeded", client.ErrUnavailable, client.ErrTimeout)
	tests := []struct {
		name         string
		setup        func(*fakeAPI)
		method       string
		target       string
		wantStatus   int
		wantTemplate string
	}{
		{"page with no search", nil, "GET", "/", 200, templates.Search},
		{"completed search", nil, "GET", searchQuery, 200, templates.Search},
		{"completed search without results", func(f *fakeAPI) { f.searchReturns(t, "search-empty.json") }, "GET", searchQuery, 200, templates.Search},
		{"invalid input", func(f *fakeAPI) { f.failSearchWith(problemFrom(t, "problem-validation.json")) }, "GET", "/?origin=XXX&date=soon", 400, templates.Search},
		{"search service failed", func(f *fakeAPI) { f.failSearchWith(fmt.Errorf("%w: status 500", client.ErrUnavailable)) }, "GET", searchQuery, 502, templates.Search},
		{"search service unreachable", func(f *fakeAPI) { f.failSearchWith(fmt.Errorf("%w: refused", client.ErrUnavailable)) }, "GET", searchQuery, 502, templates.Search},
		{"search service timed out", func(f *fakeAPI) { f.failSearchWith(timeout) }, "GET", searchQuery, 504, templates.Search},
		{"unknown path", nil, "GET", "/nowhere", 404, templates.Error},
		{"unknown nested path", nil, "GET", "/a/b/c", 404, templates.Error},
		{"api path is not proxied", nil, "GET", "/api/v1/search", 404, templates.Error},
		{"static directory", nil, "GET", "/static/", 404, templates.Error},
		{"missing static file", nil, "GET", "/static/css/missing.css", 404, templates.Error},
		{"wrong method on the page", nil, "POST", "/", 404, templates.Error},
		{"wrong method on health", nil, "POST", "/healthz", 404, templates.Error},
		{"unexpected failure in the frontend", func(f *fakeAPI) {
			f.SearchFunc = func(context.Context, string, string, string) (*client.SearchResponse, error) { panic("boom") }
		}, "GET", searchQuery, 500, templates.Error},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, "search-success.json")
			if tt.setup != nil {
				tt.setup(api)
			}
			rig := newRig(t, api)

			rec := rig.do(tt.method, tt.target)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rig.render.last(t).Name; got != tt.wantTemplate {
				t.Errorf("rendered %q, want %q", got, tt.wantTemplate)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Errorf("Content-Type = %q", ct)
			}
			if rec.Body.String() != "<rendered "+tt.wantTemplate+">" {
				t.Errorf("body = %q: it must be exactly what the renderer produced", rec.Body.String())
			}
		})
	}
}

func TestSearchPageHandsTheTemplateAViewPage(t *testing.T) {
	rig := newRig(t, newFakeAPI(t, "search-success.json"))
	rig.get(searchQuery)

	call := rig.render.last(t)
	page, ok := call.Data.(view.Page)
	if !ok {
		t.Fatalf("data is %T, want view.Page", call.Data)
	}
	wantVersion, err := assetVersion(static.FS())
	if err != nil {
		t.Fatal(err)
	}
	if page.AssetVersion != wantVersion || page.AssetVersion == "" {
		t.Errorf("AssetVersion = %q, want the digest of the embedded assets %q", page.AssetVersion, wantVersion)
	}
	if page.Results == nil || page.Results.Count != 2 || page.Form.Origin != "JFK" || page.Title != "JFK to LAX · Mar 15, 2024 · SkyPath" {
		t.Errorf("page = %+v", page)
	}
}

func TestSearchPageForwardsRequestIDToTheAPI(t *testing.T) {
	api := newFakeAPI(t, "search-success.json")
	rig := newRig(t, api)

	rec := rig.do("GET", searchQuery, "X-Request-ID", "trace-me-1")

	if got := rec.Header().Get("X-Request-ID"); got != "trace-me-1" {
		t.Errorf("response X-Request-ID = %q", got)
	}
	calls := api.searchCalls()
	if len(calls) != 1 || calls[0].RequestID != "trace-me-1" {
		t.Errorf("API calls = %+v, want the request ID forwarded", calls)
	}
}

func TestSearchPageAppliesRequestTimeout(t *testing.T) {
	tests := []struct {
		name         string
		timeout      time.Duration
		wantDeadline bool
	}{
		{"configured", 30 * time.Second, true},
		{"unset", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hasDeadline bool
			var remaining time.Duration
			api := newFakeAPI(t, "search-empty.json")
			inner := api.SearchFunc
			api.SearchFunc = func(ctx context.Context, o, d, date string) (*client.SearchResponse, error) {
				var deadline time.Time
				deadline, hasDeadline = ctx.Deadline()
				remaining = time.Until(deadline)
				return inner(ctx, o, d, date)
			}
			h := NewHandler(Dependencies{API: api, RequestTimeout: tt.timeout})

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", searchQuery, nil))

			if hasDeadline != tt.wantDeadline {
				t.Fatalf("deadline set = %v, want %v", hasDeadline, tt.wantDeadline)
			}
			if tt.wantDeadline && (remaining <= 0 || remaining > tt.timeout) {
				t.Errorf("time remaining = %v, want within %v", remaining, tt.timeout)
			}
		})
	}
}

func TestSearchPageStatusWhenTheRequestTimeoutExpires(t *testing.T) {
	api := newFakeAPI(t, "")
	// Behave like the real client: give up when the context does, and say so
	// with ErrTimeout.
	api.SearchFunc = func(ctx context.Context, _, _, _ string) (*client.SearchResponse, error) {
		<-ctx.Done()
		return nil, fmt.Errorf("%w: %w: %w", client.ErrUnavailable, client.ErrTimeout, ctx.Err())
	}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{API: api, RequestTimeout: 20 * time.Millisecond}).
		ServeHTTP(rec, httptest.NewRequest("GET", searchQuery, nil))

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", rec.Code)
	}
}

func TestErrorPagesCarryTheirViewModel(t *testing.T) {
	tests := []struct {
		name        string
		trigger     func(*testRig, *fakeAPI)
		wantStatus  int
		wantHeading string
	}{
		{"not found", func(rig *testRig, _ *fakeAPI) { rig.get("/nowhere") }, 404, "Page not found"},
		{"server error", func(rig *testRig, api *fakeAPI) {
			api.SearchFunc = func(context.Context, string, string, string) (*client.SearchResponse, error) { panic("boom") }
			rig.get(searchQuery)
		}, 500, "Something went wrong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, "search-success.json")
			rig := newRig(t, api)
			tt.trigger(rig, api)

			call := rig.render.last(t)
			page, ok := call.Data.(view.ErrorPage)
			if call.Name != templates.Error || !ok {
				t.Fatalf("rendered %q with %T, want the error template with view.ErrorPage", call.Name, call.Data)
			}
			if page.Status != tt.wantStatus || page.Heading != tt.wantHeading || page.Message == "" || page.AssetVersion == "" {
				t.Errorf("error page = %+v", page)
			}
			if page.Title != tt.wantHeading+" · SkyPath" {
				t.Errorf("Title = %q", page.Title)
			}
			if strings.Contains(strings.ToLower(page.Message), "boom") {
				t.Errorf("the cause leaked into the page: %q", page.Message)
			}
		})
	}
}

func TestRenderFailureOfThePageFallsBackToTheErrorPage(t *testing.T) {
	rig := newRig(t, newFakeAPI(t, "search-success.json"))
	rig.render.partial = "<html>half a pa"
	rig.render.fail = func(name string) error {
		if name == templates.Search {
			return errors.New("template exploded")
		}
		return nil
	}

	rec := rig.get(searchQuery)

	if rec.Code != 500 {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if rec.Body.String() != "<rendered error>" {
		t.Errorf("body = %q; the half-rendered page must not be sent", rec.Body.String())
	}
	if !strings.Contains(rig.logs.String(), "template exploded") {
		t.Errorf("render failure was not logged: %s", rig.logs.String())
	}
}

func TestRenderFailureEverywhereFallsBackToPlainText(t *testing.T) {
	tests := []struct {
		target     string
		wantStatus int
	}{
		{searchQuery, 500},
		{"/nowhere", 404},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rig := newRig(t, newFakeAPI(t, "search-success.json"))
			rig.render.partial = "<html>half"
			rig.render.fail = func(string) error { return errors.New("no templates today") }

			rec := rig.get(tt.target)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if body := rec.Body.String(); body != http.StatusText(tt.wantStatus)+"\n" {
				t.Errorf("body = %q, want the bare status text", body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("Content-Type = %q, want text/plain", ct)
			}
			if !strings.Contains(rig.logs.String(), "no templates today") {
				t.Errorf("render failure was not logged: %s", rig.logs.String())
			}
			if rec.Header().Get("Content-Security-Policy") == "" {
				t.Errorf("even the fallback needs the security headers")
			}
		})
	}
}

func TestPanicIsRecoveredAndTheServerKeepsServing(t *testing.T) {
	api := newFakeAPI(t, "search-success.json")
	rig := newRig(t, api)
	panicking := true
	good := api.SearchFunc
	api.SearchFunc = func(ctx context.Context, o, d, date string) (*client.SearchResponse, error) {
		if panicking {
			panic("kaboom")
		}
		return good(ctx, o, d, date)
	}

	rec := rig.do("GET", searchQuery, "X-Request-ID", "boom-1")
	if rec.Code != 500 || rec.Body.String() != "<rendered error>" {
		t.Errorf("status %d body %q, want the 500 error page", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Request-ID") != "boom-1" {
		t.Errorf("recovered response lost its headers: %v", rec.Header())
	}
	for _, want := range []string{`"msg":"panic recovered"`, `"request_id":"boom-1"`, "kaboom", "stack"} {
		if !strings.Contains(rig.logs.String(), want) {
			t.Errorf("log does not contain %s: %s", want, rig.logs.String())
		}
	}

	panicking = false
	if rec := rig.get(searchQuery); rec.Code != 200 {
		t.Errorf("the next request got %d, want 200", rec.Code)
	}
}

func TestSecurityHeadersOnEveryKindOfResponse(t *testing.T) {
	api := newFakeAPI(t, "search-success.json")
	version, _ := assetVersion(static.FS())
	targets := []struct {
		name   string
		method string
		target string
	}{
		{"home", "GET", "/"},
		{"search", "GET", searchQuery},
		{"search with odd input", "GET", "/?origin=%00"},
		{"not found", "GET", "/nowhere"},
		{"static versioned", "GET", "/static/css/styles.css?v=" + version},
		{"static missing", "GET", "/static/nope.js"},
		{"health", "GET", "/healthz"},
		{"head", "HEAD", "/"},
	}
	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			rig := newRig(t, api)
			rec := rig.do(tt.method, tt.target)

			h := rec.Header()
			if h.Get("X-Content-Type-Options") != "nosniff" ||
				h.Get("Referrer-Policy") != "strict-origin-when-cross-origin" ||
				h.Get("X-Frame-Options") != "DENY" {
				t.Errorf("headers = %v", h)
			}
			assertStrictCSP(t, h.Get("Content-Security-Policy"))
			if len(h.Get("X-Request-ID")) == 0 {
				t.Errorf("X-Request-ID is missing")
			}
		})
	}
}

func TestStaticAssets(t *testing.T) {
	rig := newRig(t, newFakeAPI(t, ""))
	version, err := assetVersion(static.FS())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		target    string
		wantType  string
		wantCache string
	}{
		{"stylesheet with version", "/static/css/styles.css?v=" + version, "text/css; charset=utf-8", immutableCache},
		{"script with version", "/static/js/app.js?v=" + version, "text/javascript; charset=utf-8", immutableCache},
		{"stylesheet without version", "/static/css/styles.css", "text/css; charset=utf-8", revalidateCache},
		{"stylesheet with old version", "/static/css/styles.css?v=0000", "text/css; charset=utf-8", revalidateCache},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := rig.get(tt.target)
			if rec.Code != 200 {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if rec.Body.Len() == 0 {
				t.Errorf("empty body")
			}
		})
	}

	if rig.render.count() != 0 {
		t.Errorf("serving assets rendered a template")
	}
}

func TestStaticAssetsServeTheEmbeddedFiles(t *testing.T) {
	rig := newRig(t, newFakeAPI(t, ""))
	want, err := static.FS().Open("css/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	defer want.Close()
	wantBody, _ := io.ReadAll(want)

	if got := rig.get("/static/css/styles.css").Body.Bytes(); !bytes.Equal(got, wantBody) {
		t.Errorf("served %d bytes, embedded file has %d", len(got), len(wantBody))
	}
}

func TestHealth(t *testing.T) {
	api := newFakeAPI(t, "")
	rig := newRig(t, api)

	rec := rig.get("/healthz")

	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Errorf("status %d body %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if api.airportsCalls+api.datasetCalls+len(api.searchCalls()) != 0 {
		t.Errorf("a liveness probe called the API")
	}
	if strings.Contains(rig.logs.String(), `"level":"INFO"`) {
		t.Errorf("a health probe was logged above debug: %s", rig.logs.String())
	}
}

func TestHeadRequestToThePage(t *testing.T) {
	rig := newRig(t, newFakeAPI(t, "search-success.json"))

	rec := rig.do("HEAD", "/")
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestPagesAreNotCached(t *testing.T) {
	rig := newRig(t, newFakeAPI(t, "search-success.json"))
	for _, target := range []string{"/", searchQuery, "/nowhere"} {
		if got := rig.get(target).Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", target, got)
		}
	}
}

func TestAccessLogRecordsTheOutcome(t *testing.T) {
	api := newFakeAPI(t, "")
	api.failSearchWith(fmt.Errorf("%w: down", client.ErrUnavailable))
	rig := newRig(t, api)

	rig.do("GET", searchQuery, "X-Request-ID", "log-me")

	var access map[string]any
	for _, line := range logLines(t, rig.logs) {
		if line["msg"] == "request" {
			access = line
		}
	}
	if access == nil {
		t.Fatalf("no access line in %s", rig.logs.String())
	}
	if access["status"] != float64(502) || access["path"] != "/" || access["request_id"] != "log-me" || access["bytes"] == float64(0) {
		t.Errorf("access line = %v", access)
	}
}

func TestNewHandlerRequiresAnAPI(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewHandler accepted a nil API")
		}
	}()
	NewHandler(Dependencies{})
}

func TestNewHandlerWorksWithoutLoggerOrRenderer(t *testing.T) {
	h := NewHandler(Dependencies{API: newFakeAPI(t, "search-success.json")})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// The tests below use the real templates. They are deliberately loose about
// markup, which belongs to the templates, and check only what the handler is
// responsible for.

func TestWithRealTemplates(t *testing.T) {
	h := NewHandler(Dependencies{API: newFakeAPI(t, "search-success.json")})
	version, _ := assetVersion(static.FS())

	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantBody   []string
	}{
		{"home", "/", 200, []string{"SkyPath · Flight search", "?v=" + version}},
		{"results", searchQuery, 200, []string{"JFK to LAX · Mar 15, 2024 · SkyPath"}},
		{"not found", "/nowhere", 404, []string{"Page not found", "?v=" + version}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", tt.target, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if !strings.HasPrefix(strings.ToLower(body), "<!doctype html>") {
				t.Errorf("body does not start with a doctype: %.60q", body)
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(body, want) {
					t.Errorf("body does not contain %q", want)
				}
			}
		})
	}
}

func TestWithRealTemplatesUserInputIsEscaped(t *testing.T) {
	api := newFakeAPI(t, "")
	api.failSearchWith(problemFrom(t, "problem-validation.json"))
	h := NewHandler(Dependencies{API: api})

	const payload = `"><script>alert(1)</script>`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/?origin="+url.QueryEscape(payload)+"&destination="+url.QueryEscape(payload), nil))

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<script>alert(1)</script>") {
		t.Errorf("user input reached the page unescaped")
	}
}
