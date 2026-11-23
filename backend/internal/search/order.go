package search

import (
	"cmp"
	"slices"
	"strings"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// compareItineraries orders itineraries by total travel time, then total
// price, then departure time, then flight numbers segment by segment. The
// last key exists only to settle full ties, so that the same search always
// returns the same list whatever order the source enumerated flights in.
func compareItineraries(a, b flight.Itinerary) int {
	if c := cmp.Compare(a.Duration(), b.Duration()); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Price(), b.Price()); c != 0 {
		return c
	}
	if c := a.Departure().Compare(b.Departure()); c != 0 {
		return c
	}
	return slices.CompareFunc(a.Segments, b.Segments, func(x, y flight.Flight) int {
		return strings.Compare(x.Number, y.Number)
	})
}
