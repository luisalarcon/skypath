package httpx

import (
	"log/slog"
	"math"
	"net/http"
	"time"
)

// AccessLog logs one line per request, after the response has been written,
// with the request ID, method, path, status, duration in milliseconds and
// response size in bytes. It uses the request ID stored by RequestID, so it
// belongs inside that middleware.
//
// Only the path is logged, never the query string, so search parameters stay
// out of the logs. Health probes are logged at debug level: orchestrators
// call them every few seconds and they would otherwise drown real traffic.
func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			level := slog.LevelInfo
			if isHealthProbe(r.URL.Path) {
				level = slog.LevelDebug
			}
			logger.LogAttrs(r.Context(), level, "request",
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Float64("duration_ms", durationMillis(time.Since(start))),
				slog.Int("bytes", sw.bytes),
			)
		})
	}
}

func isHealthProbe(path string) bool {
	return path == "/healthz" || path == "/readyz"
}

// durationMillis converts d to milliseconds rounded to two decimals.
func durationMillis(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}
