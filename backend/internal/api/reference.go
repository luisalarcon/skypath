package api

import "net/http"

// handleAirports lists every airport, in the order the directory returns
// them (by code).
func (s *server) handleAirports(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, newAirportsResponse(s.deps.Airports.Airports()))
}

// handleDataset describes the loaded schedule.
func (s *server) handleDataset(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, newDatasetResponse(s.deps.Dataset))
}
