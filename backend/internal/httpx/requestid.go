package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestIDHeader is the header that carries the request ID in both
// directions.
const RequestIDHeader = "X-Request-ID"

// maxRequestIDLength bounds inbound IDs so a client cannot bloat every log
// line the request produces.
const maxRequestIDLength = 64

type requestIDKey struct{}

// RequestID assigns every request an ID, stores it in the request context and
// echoes it in the response header. An inbound X-Request-ID is reused when it
// is well formed, which lets one ID follow a user action across services;
// otherwise a new one is generated. Inbound values are validated because they
// end up in logs and response bodies.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if !ValidRequestID(id) {
				id = newRequestID()
			}
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
		})
	}
}

// RequestIDFromContext returns the request ID stored by the RequestID
// middleware, or the empty string if there is none.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// ValidRequestID reports whether s is acceptable as a request ID: 1 to 64
// characters, each an ASCII letter, digit, '.', '_' or '-'.
func ValidRequestID(s string) bool {
	if s == "" || len(s) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// newRequestID returns 16 random hexadecimal characters.
func newRequestID() string {
	var b [8]byte
	rand.Read(b[:]) // never returns an error; see crypto/rand.Read
	return hex.EncodeToString(b[:])
}
