package api

import (
	"net/http"

	"github.com/luisalarcon/skypath/backend/internal/httpx"
)

const (
	contentTypeJSON    = "application/json; charset=utf-8"
	contentTypeProblem = "application/problem+json; charset=utf-8"
)

// problemKind is one entry of the error catalogue in docs/api.md: the HTTP
// status, the stable code clients switch on, and the fixed detail text.
type problemKind struct {
	status int
	code   string
	detail string
}

var (
	problemValidation = problemKind{http.StatusBadRequest, "validation_failed", "The search request is invalid."}
	problemNotFound   = problemKind{http.StatusNotFound, "not_found", "The requested resource was not found."}
	problemMethod     = problemKind{http.StatusMethodNotAllowed, "method_not_allowed", "The request method is not supported for this resource."}
	problemTimeout    = problemKind{http.StatusServiceUnavailable, "timeout", "The request took too long to complete. Please try again."}
	problemInternal   = problemKind{http.StatusInternalServerError, "internal_error", "The request could not be completed. Please try again."}
)

// problem is a problem-details document (RFC 9457).
type problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail"`
	Code      string       `json:"code"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []fieldError `json:"errors,omitempty"`
}

// fieldError describes one invalid request parameter.
type fieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeProblem answers with the problem document for kind. The type is
// "about:blank", so the title is the HTTP status phrase, as RFC 9457
// specifies.
func writeProblem(w http.ResponseWriter, r *http.Request, kind problemKind, errs []fieldError) {
	doc := problem{
		Type:      "about:blank",
		Title:     http.StatusText(kind.status),
		Status:    kind.status,
		Detail:    kind.detail,
		Code:      kind.code,
		RequestID: httpx.RequestIDFromContext(r.Context()),
		Errors:    errs,
	}
	// A problem holds only strings and integers, so encoding cannot fail.
	_ = writeBody(w, kind.status, contentTypeProblem, doc)
}

// respondInternalError is the callback handed to the recovery middleware.
func respondInternalError(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, problemInternal, nil)
}
