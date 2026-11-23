package search

import (
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// Connection limits from docs/search.md. Every bound is inclusive: a layover
// of exactly minDomesticLayover, minInternationalLayover or maxLayover is
// legal.
const (
	// minDomesticLayover is the shortest legal wait when all three airports
	// of a connection are in one country.
	minDomesticLayover = 45 * time.Minute

	// minInternationalLayover is the shortest legal wait for any connection
	// that involves a border crossing on either flight.
	minInternationalLayover = 90 * time.Minute

	// maxLayover is the longest legal wait, whatever the kind of connection.
	maxLayover = 6 * time.Hour

	// maxFlights caps an itinerary at two stops.
	maxFlights = 3
)

// connectionKind classifies the connection from a to b. It is domestic only
// when a's origin, the connecting airport and b's destination are all in one
// country. Looking at the whole journey rather than at each flight alone is
// what makes a domestic flight feeding an international one, or the reverse,
// an international connection: the passenger still has to clear a border.
func connectionKind(a, b flight.Flight) flight.ConnectionKind {
	country := a.Origin.Country
	if a.Destination.Country == country && b.Destination.Country == country {
		return flight.Domestic
	}
	return flight.International
}

// minLayover is the shortest legal wait for a connection of the given kind.
func minLayover(kind flight.ConnectionKind) time.Duration {
	if kind == flight.Domestic {
		return minDomesticLayover
	}
	return minInternationalLayover
}

// connect reports whether b can follow a in an itinerary and, if so, returns
// the layover between them. b must leave from the very airport a lands at:
// airports are matched on code, so arriving at LGA and leaving from JFK is
// never a connection even though both serve New York. The wait is measured
// between absolute instants, so it is correct whatever the two local clocks
// read.
func connect(a, b flight.Flight) (flight.Layover, bool) {
	if a.Destination.Code != b.Origin.Code {
		return flight.Layover{}, false
	}
	kind := connectionKind(a, b)
	wait := b.Departure.Sub(a.Arrival)
	if wait < minLayover(kind) || wait > maxLayover {
		return flight.Layover{}, false
	}
	return flight.Layover{Airport: a.Destination, Duration: wait, Kind: kind}, true
}
