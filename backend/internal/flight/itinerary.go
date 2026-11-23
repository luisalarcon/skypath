package flight

import (
	"errors"
	"time"
)

// ConnectionKind classifies a connection between two flights. It decides
// which minimum layover applies.
type ConnectionKind string

const (
	// Domestic means both flights stay inside one country.
	Domestic ConnectionKind = "domestic"
	// International means at least one of the two flights crosses a border.
	International ConnectionKind = "international"
)

// Layover is the wait between two consecutive flights of an itinerary.
type Layover struct {
	Airport  Airport // where the passenger waits
	Duration time.Duration
	Kind     ConnectionKind
}

// Itinerary is an ordered chain of flights from an origin to a destination.
// Layovers[i] is the connection between Segments[i] and Segments[i+1], so an
// itinerary always has one fewer layover than it has segments.
type Itinerary struct {
	Segments []Flight
	Layovers []Layover
}

// Stops is the number of connections: 0 for a direct flight.
func (it Itinerary) Stops() int {
	if len(it.Segments) == 0 {
		return 0
	}
	return len(it.Segments) - 1
}

// Departure is when the first flight leaves.
func (it Itinerary) Departure() time.Time {
	if len(it.Segments) == 0 {
		return time.Time{}
	}
	return it.Segments[0].Departure
}

// Arrival is when the last flight lands.
func (it Itinerary) Arrival() time.Time {
	if len(it.Segments) == 0 {
		return time.Time{}
	}
	return it.Segments[len(it.Segments)-1].Arrival
}

// Duration is the total travel time, from first departure to last arrival,
// including layovers.
func (it Itinerary) Duration() time.Duration {
	return it.Arrival().Sub(it.Departure())
}

// Price is the sum of the segment prices.
func (it Itinerary) Price() Money {
	var total Money
	for _, segment := range it.Segments {
		total += segment.Price
	}
	return total
}

// Query describes one itinerary search.
type Query struct {
	Origin      string // normalised airport code
	Destination string // normalised airport code
	Date        Date   // local calendar date of the first departure, at the origin
}

// ErrInvalidQuery is returned, usually wrapped, when a Query cannot be
// searched: an unknown airport, identical endpoints or a missing date.
var ErrInvalidQuery = errors.New("invalid query")
