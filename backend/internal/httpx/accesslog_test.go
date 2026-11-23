package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func newTestLogger(buf *bytes.Buffer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level}))
}

func TestAccessLogFields(t *testing.T) {
	var buf bytes.Buffer
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("short and stout"))
	}), RequestID(), AccessLog(newTestLogger(&buf, slog.LevelInfo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?origin=JFK&date=2024-03-15", nil)
	req.Header.Set(RequestIDHeader, "req-1")
	h.ServeHTTP(httptest.NewRecorder(), req)

	lines := logLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %s", len(lines), buf.String())
	}
	line := lines[0]
	want := map[string]any{
		"level":      "INFO",
		"msg":        "request",
		"request_id": "req-1",
		"method":     "GET",
		"path":       "/api/v1/search",
		"status":     float64(http.StatusTeapot),
		"bytes":      float64(len("short and stout")),
	}
	for key, w := range want {
		if line[key] != w {
			t.Errorf("%s = %v, want %v", key, line[key], w)
		}
	}
	if _, ok := line["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v, want a number", line["duration_ms"])
	}
	if strings.Contains(buf.String(), "origin=JFK") {
		t.Error("query string must not be logged")
	}
}

func TestAccessLogStatusDefaultsToOK(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		status  float64
		bytes   float64
	}{
		{"write without WriteHeader", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("abc")) }, 200, 3},
		{"no write at all", func(w http.ResponseWriter, r *http.Request) {}, 200, 0},
		{"first status wins", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.WriteHeader(http.StatusOK)
		}, 404, 0},
		{"informational status is not the response", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(http.StatusCreated)
		}, 201, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			AccessLog(newTestLogger(&buf, slog.LevelInfo))(tc.handler).
				ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

			line := logLines(t, &buf)[0]
			if line["status"] != tc.status || line["bytes"] != tc.bytes {
				t.Errorf("status, bytes = %v, %v; want %v, %v", line["status"], line["bytes"], tc.status, tc.bytes)
			}
		})
	}
}

func TestAccessLogHealthProbesAreDebug(t *testing.T) {
	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

			var info bytes.Buffer
			AccessLog(newTestLogger(&info, slog.LevelInfo))(ok).
				ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
			if info.Len() != 0 {
				t.Errorf("probe was logged at info level: %s", info.String())
			}

			var debug bytes.Buffer
			AccessLog(newTestLogger(&debug, slog.LevelDebug))(ok).
				ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
			lines := logLines(t, &debug)
			if len(lines) != 1 || lines[0]["level"] != "DEBUG" {
				t.Errorf("probe at debug level logged %v, want one DEBUG line", lines)
			}
		})
	}
}

func TestStatusWriterUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	var w http.ResponseWriter = &statusWriter{ResponseWriter: rec}

	u, ok := w.(interface{ Unwrap() http.ResponseWriter })
	if !ok {
		t.Fatal("statusWriter does not implement Unwrap")
	}
	if u.Unwrap() != http.ResponseWriter(rec) {
		t.Error("Unwrap did not return the wrapped writer")
	}
	// http.ResponseController relies on Unwrap to reach Flush.
	if err := http.NewResponseController(w).Flush(); err != nil {
		t.Errorf("Flush through the wrapper: %v", err)
	}
	if !rec.Flushed {
		t.Error("flush did not reach the underlying recorder")
	}
}
