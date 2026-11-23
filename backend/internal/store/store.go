package store

import (
	"slices"
	"strings"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// Store is a read-only, in-memory view of the flight schedule.
//
// The zero value is an empty store. Use New to build one with data.
type Store struct {
	airports map[string]flight.Airport // by code
	byCode   []flight.Airport          // every airport, ordered by code
	flights  int
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
		airports: make(map[string]flight.Airport, len(airports)),
		byCode:   make([]flight.Airport, 0, len(airports)),
		flights:  len(flights),
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
	return s
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

// Len returns the number of flights in the store.
func (s *Store) Len() int {
	return s.flights
}
