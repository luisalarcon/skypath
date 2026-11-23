package api

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

// handleReady reports readiness together with what was loaded. The schedule
// is loaded before the server starts listening, so a request that reaches
// this handler always finds the service ready.
func (s *server) handleReady(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, newReadinessResponse(s.deps.Dataset))
}

// handleHealth reports liveness: if this code runs, the process can serve.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, healthResponse{Status: "ok"})
}
