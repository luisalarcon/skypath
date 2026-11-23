package httpx

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func explode() {
	panic("kaboom")
}

func respond500(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "recovered", http.StatusInternalServerError)
}

func TestRecoverRespondsAndLogs(t *testing.T) {
	var buf bytes.Buffer
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		explode()
	}), RequestID(), Recover(newTestLogger(&buf, slog.LevelInfo), respond500))

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set(RequestIDHeader, "req-7")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "recovered" {
		t.Errorf("body = %q, want the callback's body", got)
	}
	if strings.Contains(rec.Body.String(), "kaboom") {
		t.Error("panic value leaked into the response")
	}

	lines := logLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %s", len(lines), buf.String())
	}
	line := lines[0]
	if line["level"] != "ERROR" || line["request_id"] != "req-7" || line["path"] != "/boom" || line["panic"] != "kaboom" {
		t.Errorf("unexpected log line: %v", line)
	}
	stack, _ := line["stack"].(string)
	if !strings.Contains(stack, "httpx.explode") {
		t.Errorf("stack does not include the panicking function:\n%s", stack)
	}
}

func TestRecoverKeepsServingAfterPanic(t *testing.T) {
	calls := 0
	h := Recover(newTestLogger(&bytes.Buffer{}, slog.LevelInfo), respond500)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				explode()
			}
			w.WriteHeader(http.StatusNoContent)
		}))

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))

	if first.Code != http.StatusInternalServerError || second.Code != http.StatusNoContent {
		t.Errorf("statuses = %d, %d; want 500, 204", first.Code, second.Code)
	}
}

func TestRecoverDoesNotRewriteStartedResponse(t *testing.T) {
	var buf bytes.Buffer
	called := false
	h := Recover(newTestLogger(&buf, slog.LevelInfo), func(w http.ResponseWriter, r *http.Request) {
		called = true
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		explode()
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if called {
		t.Error("respond was called after the response had started")
	}
	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want the handler's 202", rec.Code)
	}
	if len(logLines(t, &buf)) != 1 {
		t.Error("the panic should still be logged")
	}
}

func TestRecoverPassesOnErrAbortHandler(t *testing.T) {
	h := Recover(newTestLogger(&bytes.Buffer{}, slog.LevelInfo), respond500)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		}))

	defer func() {
		got := recover()
		if err, ok := got.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler to propagate", got)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestRecoverPassesThroughNormalResponses(t *testing.T) {
	h := Recover(newTestLogger(&bytes.Buffer{}, slog.LevelInfo), respond500)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("fine"))
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "fine" {
		t.Errorf("got %d %q, want 200 \"fine\"", rec.Code, rec.Body.String())
	}
}
