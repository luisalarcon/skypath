package store

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// Store is a read-only, in-memory view of the flight schedule.
//
// The zero value is an empty store. Use New to build one with data.
type Store struct {
	airports map[string]flight.Airport // by code
	byCode   []flight.Airport          // every airport, ordered by code

	// departures holds each airport's outbound flights ordered by departure
	// instant, then flight number, so a time window is a binary search away
	// and results never depend on input order.
	departures map[string][]flight.Flight
	flights    int
}

// New builds a store from validated airports and flights, as produced by the
// dataset package. Codes are used exactly as given, so they must already be
// normalised.
//
// Airport codes are expected to be unique; if one repeats, the first airport
// with that code wins, matching the rule the dataset loader applies.
//
// New copies what it keeps, so the caller may reuse or modify both slices
// afterwards without affecting the store.
func New(airports []flight.Airport, flights []flight.Flight) *Store {
	s := &Store{
		airports:   make(map[string]flight.Airport, len(airports)),
		byCode:     make([]flight.Airport, 0, len(airports)),
		departures: make(map[string][]flight.Flight),
		flights:    len(flights),
	}
	for _, a := range airports {
		if _, seen := s.airports[a.Code]; seen {
			continue
		}
		s.airports[a.Code] = a
		s.byCode = append(s.byCode, a)
	}
	slices.SortFunc(s.byCode, func(a, b flight.Airport) int {
		return strings.Compare(a.Code, b.Code)
	})

	// Appending element by element copies each flight into storage owned by
	// the store, never aliasing the caller's backing array.
	for _, f := range flights {
		code := f.Origin.Code
		s.departures[code] = append(s.departures[code], f)
	}
	for _, list := range s.departures {
		slices.SortStableFunc(list, compareDepartures)
	}
	return s
}

func compareDepartures(a, b flight.Flight) int {
	if c := a.Departure.Compare(b.Departure); c != 0 {
		return c
	}
	return strings.Compare(a.Number, b.Number)
}

// Airport returns the airport with the given code. The code must be
// normalised (see flight.NormalizeCode); lookup is exact.
func (s *Store) Airport(code string) (flight.Airport, bool) {
	a, ok := s.airports[code]
	return a, ok
}

// Airports returns every airport, ordered by code.
//
// The result is a fresh copy: the list is small and served rarely, and a copy
// means no caller can reorder or overwrite the store's own list.
func (s *Store) Airports() []flight.Airport {
	return slices.Clone(s.byCode)
}

// DeparturesFrom returns the flights leaving the airport with the given code
// at or after notBefore, earliest first; a flight departing exactly at
// notBefore is included. Flights with the same departure instant are ordered
// by flight number. An unknown code yields an empty (nil) slice.
//
// The result is a window onto the store's own list rather than a copy, so
// the call allocates nothing however often the search asks. Its capacity is
// clipped to its length, so an append by the caller reallocates instead of
// writing into the store. Callers must not modify its elements.
func (s *Store) DeparturesFrom(code string, notBefore time.Time) []flight.Flight {
	list := s.departures[code]
	start := sort.Search(len(list), func(i int) bool {
		return !list[i].Departure.Before(notBefore)
	})
	return list[start:len(list):len(list)]
}

// Len returns the number of flights in the store.
func (s *Store) Len() int {
	return s.flights
}
