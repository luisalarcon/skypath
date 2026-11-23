package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrUnavailable is wrapped by every error that means the API could not
// answer: it could not be reached, it failed, or its reply made no sense. It
// says nothing about the request itself, so callers present it as a service
// problem rather than a user mistake.
var ErrUnavailable = errors.New("search service unavailable")

// ErrTimeout is wrapped, together with ErrUnavailable, when the API did not
// answer in time. It lets callers tell "too slow" from "broken".
var ErrTimeout = errors.New("timed out")

// FieldError is one problem with one search parameter.
type FieldError struct {
	Field   string `json:"field"`   // "origin", "destination" or "date"
	Code    string `json:"code"`    // stable identifier, e.g. "unknown_airport"
	Message string `json:"message"` // written for end users and safe to display
}

// ProblemError is a problem-details response (RFC 9457) to a request the API
// rejected with a 4xx status. For validation failures Errors lists every
// invalid field.
type ProblemError struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail"`
	Code      string       `json:"code"` // stable identifier, e.g. "validation_failed"
	RequestID string       `json:"requestId"`
	Errors    []FieldError `json:"errors"`
}

// Error describes the problem for logs; use Errors for text meant for users.
func (e *ProblemError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "search API rejected the request: %d %s", e.Status, e.Code)
	if e.Detail != "" {
		b.WriteString(": " + e.Detail)
	}
	return b.String()
}

// unavailable wraps a failure to get a usable answer. A deadline or network
// timeout additionally wraps ErrTimeout.
func unavailable(cause error) error {
	if isTimeout(cause) {
		return fmt.Errorf("%w: %w: %w", ErrUnavailable, ErrTimeout, cause)
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, cause)
}

func isTimeout(err error) bool {
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
