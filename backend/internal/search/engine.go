package search

import (
	"context"
	"fmt"
	"slices"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// Engine finds itineraries among the flights of a FlightSource.
//
// An Engine holds nothing but its source: every search keeps its state on
// its own stack. It is therefore safe for concurrent use whenever the source
// is, and the source's contract (read-only, callers never modify returned
// slices) makes that the normal case.
type Engine struct {
	src FlightSource
}

// NewEngine returns an Engine that reads airports and flights from src.
func NewEngine(src FlightSource) *Engine {
	return &Engine{src: src}
}

// Search returns every itinerary of one to three flights from q.Origin to
// q.Destination whose first flight departs on q.Date in the origin
// airport's local time. Later flights may depart on following days.
//
// An unknown airport, identical origin and destination or a zero date
// yields an error wrapping flight.ErrInvalidQuery. A valid query with no
// matches yields an empty, non-nil slice and a nil error.
//
// Each returned itinerary owns its Segments and Layovers slices.
func (e *Engine) Search(ctx context.Context, q flight.Query) ([]flight.Itinerary, error) {
	origin, err := e.validate(q)
	if err != nil {
		return nil, err
	}

	// The search date is a half-open range of instants in the origin's zone.
	// Deriving the end from the next date, rather than adding 24 hours,
	// keeps it right on days when the clocks change.
	dayStart := q.Date.Start(origin.Location)
	dayEnd := q.Date.AddDays(1).Start(origin.Location)

	w := walk{src: e.src, destination: q.Destination, results: []flight.Itinerary{}}

	// Scratch buffers for the walk. Sibling branches overwrite each other's
	// tail in place, which is safe because record copies a finished path out
	// before the next branch runs.
	path := make([]flight.Flight, 0, maxFlights)
	layovers := make([]flight.Layover, 0, maxFlights-1)
	visited := make([]string, 0, maxFlights+1)

	for _, f := range e.src.DeparturesFrom(origin.Code, dayStart) {
		if !f.Departure.Before(dayEnd) {
			break
		}
		if f.Destination.Code == origin.Code {
			continue
		}
		w.extend(append(path, f), layovers, append(visited, origin.Code, f.Destination.Code))
	}

	return w.results, nil
}

// validate checks q against the source and returns the origin airport, whose
// zone defines the search date.
func (e *Engine) validate(q flight.Query) (flight.Airport, error) {
	origin, ok := e.src.Airport(q.Origin)
	if !ok {
		return flight.Airport{}, fmt.Errorf("%w: unknown airport code %q", flight.ErrInvalidQuery, q.Origin)
	}
	if _, ok := e.src.Airport(q.Destination); !ok {
		return flight.Airport{}, fmt.Errorf("%w: unknown airport code %q", flight.ErrInvalidQuery, q.Destination)
	}
	if q.Origin == q.Destination {
		return flight.Airport{}, fmt.Errorf("%w: origin and destination must be different", flight.ErrInvalidQuery)
	}
	if q.Date.IsZero() {
		return flight.Airport{}, fmt.Errorf("%w: date is required", flight.ErrInvalidQuery)
	}
	return origin, nil
}

// walk is the state of a single search. It exists only for the duration of
// one Search call.
type walk struct {
	src         FlightSource
	destination string
	results     []flight.Itinerary
}

// extend continues path, a legal chain of flights whose connections are
// described by layovers, and records every way it can reach the destination.
// visited holds every airport the path has been through, origin included.
func (w *walk) extend(path []flight.Flight, layovers []flight.Layover, visited []string) {
	last := path[len(path)-1]

	if last.Destination.Code == w.destination {
		w.record(path, layovers)
		return
	}
	if len(path) == maxFlights {
		return
	}

	// Ask only for flights inside the widest window any connection could
	// use. connect then applies the minimum for this connection's kind.
	earliest := last.Arrival.Add(min(minDomesticLayover, minInternationalLayover))
	latest := last.Arrival.Add(maxLayover)

	for _, g := range w.src.DeparturesFrom(last.Destination.Code, earliest) {
		if g.Departure.After(latest) {
			break
		}
		if slices.Contains(visited, g.Destination.Code) {
			continue
		}
		layover, ok := connect(last, g)
		if !ok {
			continue
		}
		w.extend(append(path, g), append(layovers, layover), append(visited, g.Destination.Code))
	}
}

// record stores a finished path as an itinerary. path and layovers are
// scratch buffers shared with the rest of the walk, so the itinerary gets
// copies of its own.
func (w *walk) record(path []flight.Flight, layovers []flight.Layover) {
	w.results = append(w.results, flight.Itinerary{
		Segments: slices.Clone(path),
		Layovers: slices.Clone(layovers),
	})
}
