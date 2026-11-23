package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	logger, _ := testLogger()
	return NewHandler(Dependencies{Logger: logger})
}

func TestHealthz(t *testing.T) {
	rec := get(newTestHandler(t), "/healthz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got, want := decodeBody(t, rec), map[string]any{"status": "ok"}; !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

func TestHeadIsServedWhereGetIs(t *testing.T) {
	rec := do(newTestHandler(t), http.MethodHead, "/healthz")
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD /healthz status = %d, want 200", rec.Code)
	}
}

func TestNotFound(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{"/", "/nope", "/api", "/api/v1", "/api/v1/search/", "/api/v1/flights", "/healthz/extra", "/HEALTHZ"} {
		t.Run(path, func(t *testing.T) {
			rec := get(h, path, "X-Request-ID", "req-404")

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			want := map[string]any{
				"type":      "about:blank",
				"title":     "Not Found",
				"status":    float64(404),
				"detail":    "The requested resource was not found.",
				"code":      "not_found",
				"requestId": "req-404",
			}
			if got := decodeBody(t, rec); !reflect.DeepEqual(got, want) {
				t.Errorf("body = %v, want %v", got, want)
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			rec := do(h, method, "/healthz", "X-Request-ID", "req-405")

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", rec.Code)
			}
			if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
				t.Errorf("Allow = %q, want %q", got, "GET, HEAD")
			}
			if got := rec.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			want := map[string]any{
				"type":      "about:blank",
				"title":     "Method Not Allowed",
				"status":    float64(405),
				"detail":    "The request method is not supported for this resource.",
				"code":      "method_not_allowed",
				"requestId": "req-405",
			}
			if got := decodeBody(t, rec); !reflect.DeepEqual(got, want) {
				t.Errorf("body = %v, want %v", got, want)
			}
		})
	}
}

func TestRequestIDHeader(t *testing.T) {
	h := newTestHandler(t)
	generated := regexp.MustCompile(`^[0-9a-f]{16}$`)

	tests := []struct {
		name    string
		inbound string // empty means no header
		keep    bool
	}{
		{"valid ID is echoed", "3f2a9c1e7b4d4a10", true},
		{"missing ID is generated", "", false},
		{"invalid characters are replaced", "bad id; drop table", false},
		{"overlong ID is replaced", strings.Repeat("a", 65), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var headers []string
			if tc.inbound != "" {
				headers = []string{"X-Request-ID", tc.inbound}
			}
			// A problem response proves the ID reaches the body as well.
			rec := get(h, "/nope", headers...)

			id := rec.Header().Get("X-Request-ID")
			if tc.keep && id != tc.inbound {
				t.Errorf("X-Request-ID = %q, want %q", id, tc.inbound)
			}
			if !tc.keep && !generated.MatchString(id) {
				t.Errorf("X-Request-ID = %q, want a generated 16-hex-digit ID", id)
			}
			if got := decodeBody(t, rec)["requestId"]; got != id {
				t.Errorf("body requestId = %v, want %q", got, id)
			}
		})
	}
}

func TestResponseHeaders(t *testing.T) {
	h := newTestHandler(t)
	for _, target := range []string{"/healthz", "/nope"} {
		t.Run(target, func(t *testing.T) {
			rec := get(h, target)
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestAccessLogLine(t *testing.T) {
	logger, buf := testLogger()
	h := NewHandler(Dependencies{Logger: logger})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope", nil))

	records := logRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("got %d log lines, want 1", len(records))
	}
	rec := records[0]
	if rec["msg"] != "request" || rec["path"] != "/nope" || rec["status"] != float64(404) || rec["method"] != "GET" {
		t.Errorf("unexpected access log line: %v", rec)
	}
	if id, _ := rec["request_id"].(string); id == "" {
		t.Error("access log line has no request_id")
	}
}
