package flight

import "time"

// Flight is a single scheduled flight between two airports.
//
// Departure and Arrival are absolute instants. Each carries the time zone of
// its own airport, so formatting one yields that airport's local time while
// arithmetic between them is always correct regardless of zones, daylight
// saving or the date line.
type Flight struct {
	Number      string // e.g. "SP101"
	Airline     string
	Aircraft    string
	Origin      Airport
	Destination Airport
	Departure   time.Time
	Arrival     time.Time
	Price       Money
}

// Duration is the time in the air: arrival minus departure.
func (f Flight) Duration() time.Duration {
	return f.Arrival.Sub(f.Departure)
}

// Domestic reports whether the flight starts and ends in the same country.
func (f Flight) Domestic() bool {
	return f.Origin.Country == f.Destination.Country
}
