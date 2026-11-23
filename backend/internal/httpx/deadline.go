package httpx

import (
	"context"
	"net/http"
	"time"
)

// Deadline gives every request a time limit by putting a deadline on its
// context. Handlers that honour the context stop work and report
// context.DeadlineExceeded, and can then answer with an error of their own.
//
// It deliberately does not use http.TimeoutHandler: that buffers the whole
// response and writes its own body, which would bypass the API's error
// format. A zero or negative d means no limit.
func Deadline(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		if d <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
