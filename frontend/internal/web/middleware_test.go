package web

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/luisalarcon/skypath/frontend/internal/client"
)

// newJSONLogger returns a logger writing JSON lines at the named level.
func newJSONLogger(w io.Writer, level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		panic(err)
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l}))
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		wantSame bool
	}{
		{"absent", "", false},
		{"simple", "abc123", true},
		{"all allowed punctuation", "a.b_c-d", true},
		{"upstream style", "3f2a9c1e7b4d4a10", true},
		{"longest allowed", strings.Repeat("a", 64), true},
		{"too long", strings.Repeat("a", 65), false},
		{"space", "has space", false},
		{"slash", "a/b", false},
		{"quote", `a"b`, false},
		{"non ASCII", "idé", false},
		{"comma", "a,b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := requestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = client.RequestID(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("X-Request-ID", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Request-ID")
			if got != seen {
				t.Errorf("response header %q differs from context ID %q", got, seen)
			}
			if tt.wantSame && got != tt.header {
				t.Errorf("ID = %q, want the caller's %q", got, tt.header)
			}
			if !tt.wantSame && !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(got) {
				t.Errorf("ID = %q, want 16 generated hex characters", got)
			}
		})
	}
}

func TestGeneratedRequestIDsDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newRequestID()
		if seen[id] {
			t.Fatalf("duplicate request ID %q", id)
		}
		seen[id] = true
	}
}

// logLines decodes the JSON log lines written to buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line %q is not JSON: %v", raw, err)
		}
		lines = append(lines, line)
	}
	return lines
}

func TestAccessLog(t *testing.T) {
	var buf bytes.Buffer
	logger := newJSONLogger(&buf, "debug")
	h := requestID(accessLog(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	})))

	req := httptest.NewRequest(http.MethodGet, "/some/path?origin=JFK&secret=1", nil)
	req.Header.Set("X-Request-ID", "rid-42")
	h.ServeHTTP(httptest.NewRecorder(), req)

	lines := logLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1: %s", len(lines), buf.String())
	}
	line := lines[0]
	want := map[string]any{
		"level":      "INFO",
		"msg":        "request",
		"request_id": "rid-42",
		"method":     "GET",
		"path":       "/some/path",
		"status":     float64(http.StatusTeapot),
		"bytes":      float64(len("short and stout")),
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %v", k, line[k], v)
		}
	}
	if d, ok := line["duration_ms"].(float64); !ok || d < 0 {
		t.Errorf("duration_ms = %v, want a non-negative number", line["duration_ms"])
	}
	if strings.Contains(buf.String(), "secret") || strings.Contains(buf.String(), "JFK") {
		t.Errorf("the query string leaked into the log: %s", buf.String())
	}
}

func TestAccessLogImplicitStatus(t *testing.T) {
	var buf bytes.Buffer
	h := accessLog(newJSONLogger(&buf, "info"), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got := logLines(t, &buf)[0]["status"]; got != float64(200) {
		t.Errorf("status = %v, want 200 for a handler that only writes", got)
	}
}

func TestAccessLogHealthChecksAreDebug(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	var quiet bytes.Buffer
	accessLog(newJSONLogger(&quiet, "info"), handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if quiet.Len() != 0 {
		t.Errorf("health probe logged at info: %s", quiet.String())
	}

	var verbose bytes.Buffer
	accessLog(newJSONLogger(&verbose, "debug"), handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if lines := logLines(t, &verbose); len(lines) != 1 || lines[0]["level"] != "DEBUG" {
		t.Errorf("health probe at debug level = %v", lines)
	}
}

func failedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed page", http.StatusInternalServerError)
	})
}

func TestRecoverPanicsAnswersWithTheFailurePage(t *testing.T) {
	var buf bytes.Buffer
	logger := newJSONLogger(&buf, "info")
	h := requestID(accessLog(logger, recoverPanics(logger, failedHandler(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom: secret detail")
	}))))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "rid-7")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || rec.Body.String() != "failed page\n" {
		t.Errorf("status %d body %q, want the failure page", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("the panic value reached the visitor")
	}

	lines := logLines(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("log lines = %d, want the panic and the access line: %s", len(lines), buf.String())
	}
	panicLine := lines[0]
	if panicLine["level"] != "ERROR" || panicLine["request_id"] != "rid-7" || panicLine["panic"] != "kaboom: secret detail" {
		t.Errorf("panic line = %v", panicLine)
	}
	if stack, _ := panicLine["stack"].(string); !strings.Contains(stack, "middleware_test.go") {
		t.Errorf("stack does not point at the panic: %q", stack)
	}
	if lines[1]["status"] != float64(500) {
		t.Errorf("access line status = %v, want 500", lines[1]["status"])
	}
}

func TestRecoverPanicsAfterPartialResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := newJSONLogger(&buf, "info")
	h := recoverPanics(logger, failedHandler(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("late")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "partial" {
		t.Errorf("status %d body %q; a started response must not get a second body", rec.Code, rec.Body.String())
	}
	if !strings.Contains(buf.String(), "panic recovered") {
		t.Errorf("the panic was not logged")
	}
}

func TestRecoverPanicsLetsAbortHandlerThrough(t *testing.T) {
	h := recoverPanics(newJSONLogger(&bytes.Buffer{}, "info"), failedHandler(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler to propagate", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestRecoverPanicsPassesThroughNormalResponses(t *testing.T) {
	h := recoverPanics(newJSONLogger(&bytes.Buffer{}, "info"), failedHandler(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fine"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "fine" {
		t.Errorf("status %d body %q", rec.Code, rec.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
	}
	for name, value := range want {
		if got := rec.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
	assertStrictCSP(t, rec.Header().Get("Content-Security-Policy"))
}

// assertStrictCSP checks the policy promised by docs/frontend.md.
func assertStrictCSP(t *testing.T, csp string) {
	t.Helper()
	if csp == "" {
		t.Fatal("Content-Security-Policy is missing")
	}
	for _, banned := range []string{"unsafe-inline", "unsafe-eval", "*", "http:", "https:", "data:"} {
		if strings.Contains(csp, banned) {
			t.Errorf("policy %q must not contain %q", csp, banned)
		}
	}
	directives := map[string]string{}
	for _, part := range strings.Split(csp, ";") {
		name, value, _ := strings.Cut(strings.TrimSpace(part), " ")
		directives[name] = value
	}
	for name, value := range map[string]string{
		"default-src":     "'self'",
		"script-src":      "'self'",
		"style-src":       "'self'",
		"img-src":         "'self'",
		"form-action":     "'self'",
		"frame-ancestors": "'none'",
	} {
		if directives[name] != value {
			t.Errorf("%s = %q, want %q", name, directives[name], value)
		}
	}
}
