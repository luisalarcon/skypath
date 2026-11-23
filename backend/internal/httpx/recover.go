package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover turns a panic in a downstream handler into a logged error and an
// error response, so one bad request cannot take the process down.
//
// The panic value and stack are written to the log with the request ID; they
// never reach the client. respond writes the error body and is only called if
// the handler had not already started the response, since headers cannot be
// changed after that.
//
// http.ErrAbortHandler is passed on: net/http uses it to abort a response on
// purpose and expects it to reach the server.
func Recover(logger *slog.Logger, respond func(w http.ResponseWriter, r *http.Request)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.LogAttrs(r.Context(), slog.LevelError, "panic recovered",
					slog.String("request_id", RequestIDFromContext(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				if !sw.wroteHeader {
					respond(w, r)
				}
			}()
			next.ServeHTTP(sw, r)
		})
	}
}
