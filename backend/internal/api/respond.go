package api

import (
	"encoding/json"
	"net/http"

	"github.com/luisalarcon/skypath/backend/internal/httpx"
)

// writeBody encodes v completely before writing anything, so an encoding
// failure can still be answered with a clean error instead of a truncated
// body behind a 200. It reports only encoding failures: a failed write means
// the client has gone, and there is nobody left to tell.
func writeBody(w http.ResponseWriter, status int, contentType string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
	return nil
}

// writeJSON answers 200 with v encoded as JSON.
func (s *server) writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	if err := writeBody(w, http.StatusOK, contentTypeJSON, v); err != nil {
		s.internalError(w, r, "encode response", err)
	}
}

// internalError logs the cause and answers with the generic 500 problem. The
// cause never reaches the client; the shared request ID lets an operator find
// it from the response.
func (s *server) internalError(w http.ResponseWriter, r *http.Request, msg string, cause error) {
	s.logger.Error(msg,
		"request_id", httpx.RequestIDFromContext(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
		"error", cause,
	)
	writeProblem(w, r, problemInternal, nil)
}
