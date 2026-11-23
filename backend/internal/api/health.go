package api

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth reports liveness: if this code runs, the process can serve.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, healthResponse{Status: "ok"})
}
