package web

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/luisalarcon/skypath/frontend/internal/client"
)

// requestIDHeader carries the request ID in both directions: from the
// caller, and onward to the API.
const requestIDHeader = "X-Request-ID"

// contentSecurityPolicy allows scripts, styles, images and requests from this
// origin only. Without 'unsafe-inline' the browser refuses inline script and
// style, so markup injected into the page cannot run or restyle anything.
// frame-ancestors forbids embedding, form-action keeps submissions on this
// origin, and base-uri stops an injected <base> from redirecting relative
// URLs.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self'; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// validRequestID matches the IDs the API also accepts: short and free of
// characters that could confuse a log line or a header.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// requestID gives every request an ID, taken from the caller's header when it
// is well formed and generated otherwise. The ID is returned in the response,
// stored in the request context for the logs and forwarded to the API.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(client.WithRequestID(r.Context(), id)))
	})
}

// newRequestID returns 16 random hexadecimal characters.
func newRequestID() string {
	var b [8]byte
	// crypto/rand.Read does not fail on supported platforms; since Go 1.24 it
	// terminates the program rather than return an error.
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// recorder remembers what a handler wrote, for the access log and for
// deciding whether an error page can still be sent.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

// record returns w as a recorder, wrapping it only if an outer middleware has
// not already, so every layer observes the same state.
func record(w http.ResponseWriter) *recorder {
	if rec, ok := w.(*recorder); ok {
		return rec
	}
	return &recorder{ResponseWriter: w}
}

func (r *recorder) WriteHeader(status int) {
	if !r.wrote {
		r.status, r.wrote = status, true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(p []byte) (int, error) {
	if !r.wrote {
		r.status, r.wrote = http.StatusOK, true
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// accessLog writes one line per request. Health probes are logged at debug so
// they do not drown out real traffic. The query string is left out of the
// line: it holds what visitors searched for.
func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := record(w)
		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if r.URL.Path == "/healthz" {
			level = slog.LevelDebug
		}
		elapsed := float64(time.Since(start).Microseconds()) / 1000
		logger.LogAttrs(r.Context(), level, "request",
			slog.String("request_id", client.RequestID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Float64("duration_ms", math.Round(elapsed*100)/100),
			slog.Int("bytes", rec.bytes),
		)
	})
}

// recoverPanics keeps a panic in one request from taking the process down. The
// panic is logged with its stack and, if nothing has been sent yet, the
// visitor gets failed instead of a dropped connection. The cause is never
// shown to the visitor.
func recoverPanics(logger *slog.Logger, failed http.Handler, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := record(w)
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			// ErrAbortHandler is net/http's way of ending a response on
			// purpose; it must reach the server to close the connection.
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v)
			}
			logger.Error("panic recovered",
				"request_id", client.RequestID(r.Context()),
				"panic", fmt.Sprint(v),
				"stack", string(debug.Stack()))
			if !rec.wrote {
				failed.ServeHTTP(rec, r)
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

// securityHeaders sets the browser protections on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
