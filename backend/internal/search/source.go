// Package search finds itineraries: chains of one to three flights joined by
// legal connections. It owns the connection rules (minimum and maximum
// layover, domestic versus international) and the search algorithm.
//
// The package reads flight data only through FlightSource, so it can be
// tested with a handful of hand-made flights and does not care whether the
// data lives in memory or in a database.
//
// Public API:
//
//	NewEngine(src FlightSource) *Engine
//	(*Engine).Search(ctx context.Context, q flight.Query) ([]flight.Itinerary, error)
//
// Search returns itineraries ordered by total duration, then price, then
// departure time, then flight numbers. An unknown airport, identical origin
// and destination or a zero date yields an error wrapping
// flight.ErrInvalidQuery. A valid query with no matches yields an empty,
// non-nil slice and a nil error.
package search

import (
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// FlightSource is the read-only view of flight data the search needs.
type FlightSource interface {
	// Airport returns the airport with the given IATA code.
	Airport(code string) (flight.Airport, bool)

	// DeparturesFrom returns the flights leaving the given airport at or
	// after notBefore, ordered by departure time, earliest first. It returns
	// an empty slice for an unknown airport. Callers must not modify the
	// returned slice.
	DeparturesFrom(code string, notBefore time.Time) []flight.Flight
}
