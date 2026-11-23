// Package api is the HTTP layer of the flight search service: routing,
// request validation, JSON encoding and error responses. It contains no
// search logic; it depends on the small interfaces below and is wired to
// real implementations in cmd/server.
//
// The wire format is defined by api/openapi.yaml at the repository root.
//
// Public API:
//
//	NewHandler(deps Dependencies) http.Handler
//
// NewHandler returns the complete handler for the service: every route with
// the full middleware chain already applied, ready to hand to an http.Server.
package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// Dependencies is everything the HTTP layer needs from the rest of the
// service.
type Dependencies struct {
	Search   Searcher
	Airports AirportDirectory
	Dataset  DatasetInfo
	Logger   *slog.Logger

	// RequestTimeout bounds the handling of a single request. Zero means no
	// limit.
	RequestTimeout time.Duration
}

// Searcher finds itineraries for a validated query.
type Searcher interface {
	Search(ctx context.Context, q flight.Query) ([]flight.Itinerary, error)
}

// AirportDirectory answers questions about known airports. The handler uses
// it to validate codes and to serve the airport list.
type AirportDirectory interface {
	// Airport returns the airport with the given IATA code.
	Airport(code string) (flight.Airport, bool)
	// Airports returns every airport, ordered by code.
	Airports() []flight.Airport
}

// DatasetInfo summarises the loaded schedule. It is computed once at startup
// and never changes while the process runs.
type DatasetInfo struct {
	Airports       int
	Flights        int
	FirstDeparture flight.Date // earliest local departure date in the schedule
	LastDeparture  flight.Date // latest local departure date in the schedule
	Skipped        []SkippedRecord
}

// SkippedRecord describes one source record that was rejected at load time.
type SkippedRecord struct {
	Record string // what was skipped, e.g. "flight SP995"
	Reason string // why, in plain language
}
