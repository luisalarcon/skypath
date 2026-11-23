package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/luisalarcon/skypath/backend/internal/flight"
	"github.com/luisalarcon/skypath/backend/internal/httpx"
)

// handleSearch validates the request, runs the search and encodes the result.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	req, errs := validateSearch(r.URL.Query(), s.deps.Airports)
	if len(errs) > 0 {
		writeProblem(w, r, problemValidation, errs)
		return
	}

	itineraries, err := s.deps.Search.Search(r.Context(), req.query)
	if err != nil {
		s.searchFailed(w, r, err)
		return
	}

	s.writeJSON(w, r, newSearchResponse(req.origin, req.destination, req.query.Date, itineraries))
}

// searchFailed maps an error from the searcher to a problem response.
func (s *server) searchFailed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, flight.ErrInvalidQuery):
		// The handler validates before searching, so reaching this means the
		// searcher rejected something the handler accepted. The client still
		// sent a bad request as far as it can tell, but the mismatch is ours
		// to look at.
		s.logger.Warn("search rejected a validated query",
			"request_id", httpx.RequestIDFromContext(r.Context()), "error", err)
		writeProblem(w, r, problemValidation, nil)
	case errors.Is(err, context.DeadlineExceeded):
		s.logger.Warn("search exceeded the request deadline",
			"request_id", httpx.RequestIDFromContext(r.Context()), "error", err)
		writeProblem(w, r, problemTimeout, nil)
	default:
		s.internalError(w, r, "search failed", err)
	}
}
