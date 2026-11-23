// Package store holds the flight schedule in memory, indexed for the two
// questions the service asks: "what is this airport?" and "which flights
// leave this airport at or after this instant?".
//
// A Store is built once and never modified, so any number of requests can
// read it concurrently without locking.
//
// Public API:
//
//	New(airports []flight.Airport, flights []flight.Flight) *Store
//	(*Store).Airport(code string) (flight.Airport, bool)
//	(*Store).Airports() []flight.Airport // ordered by code
//	(*Store).DeparturesFrom(code string, notBefore time.Time) []flight.Flight
//	(*Store).Len() int // number of flights
//
// *Store satisfies search.FlightSource and api.AirportDirectory. The
// compile-time check belongs in cmd/server, the only package that knows all
// three, so that this package depends on neither consumer.
package store
