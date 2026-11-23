package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const problemType = "application/problem+json; charset=utf-8"

func TestProblemExampleIsFullyModelled(t *testing.T) {
	assertNoUnknownFields(t, "problem-validation.json", &ProblemError{})
	assertNoUnknownFields(t, "problem-internal.json", &ProblemError{})
}

func TestSearchDecodesValidationProblem(t *testing.T) {
	c := newTestClient(t, serveExample(t, 400, problemType, "problem-validation.json"))

	res, err := c.Search(context.Background(), "XXX", "LAX", "soon")
	if res != nil {
		t.Errorf("response = %+v, want nil", res)
	}

	var problem *ProblemError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *ProblemError", err)
	}
	if errors.Is(err, ErrUnavailable) {
		t.Errorf("a rejected request must not look like an unavailable service")
	}
	if problem.Status != 400 || problem.Title != "Bad Request" || problem.Code != "validation_failed" ||
		problem.Detail != "The search request is invalid." || problem.RequestID != "3f2a9c1e7b4d4a10" || problem.Type != "about:blank" {
		t.Errorf("problem decoded wrongly: %+v", problem)
	}
	want := []FieldError{
		{Field: "origin", Code: "unknown_airport", Message: "Unknown airport code XXX."},
		{Field: "date", Code: "invalid_format", Message: "Date must be a valid date in YYYY-MM-DD format."},
	}
	if len(problem.Errors) != len(want) {
		t.Fatalf("field errors = %+v, want %+v", problem.Errors, want)
	}
	for i := range want {
		if problem.Errors[i] != want[i] {
			t.Errorf("field error %d = %+v, want %+v", i, problem.Errors[i], want[i])
		}
	}
}

func TestProblemWithoutCharsetParameter(t *testing.T) {
	c := newTestClient(t, serveExample(t, 400, "application/problem+json", "problem-validation.json"))

	_, err := c.Search(context.Background(), "XXX", "LAX", "soon")
	var problem *ProblemError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *ProblemError", err)
	}
}

func TestOtherClientProblemsAreProblemErrors(t *testing.T) {
	body := `{"type":"about:blank","title":"Not Found","status":404,"detail":"The requested resource was not found.","code":"not_found"}`
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", problemType)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(body))
	}))

	_, err := c.Airports(context.Background())
	var problem *ProblemError
	if !errors.As(err, &problem) || problem.Status != 404 || problem.Code != "not_found" || len(problem.Errors) != 0 {
		t.Errorf("error = %v, want a 404 not_found *ProblemError", err)
	}
}

func TestProblemErrorMessage(t *testing.T) {
	err := &ProblemError{Status: 400, Code: "validation_failed", Detail: "The search request is invalid."}
	want := "search API rejected the request: 400 validation_failed: The search request is invalid."
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	if got := (&ProblemError{Status: 404, Code: "not_found"}).Error(); strings.HasSuffix(got, ": ") {
		t.Errorf("Error() = %q has a dangling separator", got)
	}
}

func TestClientErrorWithoutProblemBodyIsUnavailable(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{"plain text", "text/plain", "nope"},
		{"json that is not a problem", "application/json", `{"error":"nope"}`},
		{"malformed problem", problemType, `{"status": `},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tt.body))
			}))
			_, err := c.Search(context.Background(), "JFK", "LAX", "2024-03-15")

			var problem *ProblemError
			if errors.As(err, &problem) || !errors.Is(err, ErrUnavailable) {
				t.Errorf("error = %v, want ErrUnavailable and no *ProblemError", err)
			}
		})
	}
}

func TestServerFailuresAreUnavailableButNotTimeouts(t *testing.T) {
	c := newTestClient(t, serveExample(t, 500, problemType, "problem-internal.json"))

	_, err := c.Search(context.Background(), "JFK", "LAX", "2024-03-15")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if errors.Is(err, ErrTimeout) {
		t.Errorf("a 500 must not be reported as a timeout")
	}
	if msg := err.Error(); !strings.Contains(msg, "500") || !strings.Contains(msg, "3f2a9c1e7b4d4a10") {
		t.Errorf("error %q should carry the status and the API's request ID", msg)
	}
	var problem *ProblemError
	if errors.As(err, &problem) {
		t.Errorf("a 5xx must not be a *ProblemError")
	}
}

func TestBackendTimeoutResponsesAreTimeouts(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		ctype  string
	}{
		{
			"api time limit",
			http.StatusServiceUnavailable,
			`{"type":"about:blank","title":"Service Unavailable","status":503,"code":"timeout","requestId":"abc"}`,
			problemType,
		},
		{"gateway timeout", http.StatusGatewayTimeout, "", "text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.ctype)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			_, err := c.Search(context.Background(), "JFK", "LAX", "2024-03-15")
			if !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrTimeout) {
				t.Errorf("error = %v, want ErrUnavailable and ErrTimeout", err)
			}
		})
	}
}

// blockUntilCancelled holds a request open until the client gives up.
func blockUntilCancelled() http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	})
}

func TestSlowServerTimesOut(t *testing.T) {
	t.Run("http client timeout", func(t *testing.T) {
		c := newTestClient(t, blockUntilCancelled())
		c.http.Timeout = 50 * time.Millisecond

		_, err := c.Search(context.Background(), "JFK", "LAX", "2024-03-15")
		if !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrTimeout) {
			t.Errorf("error = %v, want ErrUnavailable and ErrTimeout", err)
		}
	})

	t.Run("context deadline", func(t *testing.T) {
		c := newTestClient(t, blockUntilCancelled())
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := c.Airports(ctx)
		if !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrTimeout) {
			t.Errorf("error = %v, want ErrUnavailable and ErrTimeout", err)
		}
	})
}

func TestNonTimeoutFailuresAreNotTimeouts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := newTestClient(t, blockUntilCancelled())
	_, err := c.Dataset(ctx)
	if errors.Is(err, ErrTimeout) {
		t.Errorf("a cancelled request must not be reported as a timeout: %v", err)
	}
}
