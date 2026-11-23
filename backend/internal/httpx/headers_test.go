package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecureHeaders(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"success", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }},
		{"error status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{"no write", func(w http.ResponseWriter, r *http.Request) {}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			SecureHeaders()(tc.handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestSecureHeadersSurviveRecover(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}), Recover(newTestLogger(&bytes.Buffer{}, slog.LevelInfo), respond500), SecureHeaders())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("recovered response lost its headers: %v", rec.Header())
	}
}
