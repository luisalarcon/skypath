package search

import (
	"fmt"
	"slices"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"

	// Tests resolve real time zones; embedding the database keeps them
	// independent of what the host has installed.
	_ "time/tzdata"
)

// Airports shared by the tests. Each carries its real IANA zone, so every
// time written below goes through genuine zone arithmetic: daylight saving,
// Phoenix's lack of it and the date line all apply.
var (
	jfk = newAirport("JFK", "New York", "US", "America/New_York")
	lga = newAirport("LGA", "New York", "US", "America/New_York")
	bos = newAirport("BOS", "Boston", "US", "America/New_York")
	mia = newAirport("MIA", "Miami", "US", "America/New_York")
	ord = newAirport("ORD", "Chicago", "US", "America/Chicago")
	den = newAirport("DEN", "Denver", "US", "America/Denver")
	phx = newAirport("PHX", "Phoenix", "US", "America/Phoenix")
	lax = newAirport("LAX", "Los Angeles", "US", "America/Los_Angeles")
	sfo = newAirport("SFO", "San Francisco", "US", "America/Los_Angeles")
	sea = newAirport("SEA", "Seattle", "US", "America/Los_Angeles")
	lhr = newAirport("LHR", "London", "GB", "Europe/London")
	cdg = newAirport("CDG", "Paris", "FR", "Europe/Paris")
	mex = newAirport("MEX", "Mexico City", "MX", "America/Mexico_City")
	nrt = newAirport("NRT", "Tokyo", "JP", "Asia/Tokyo")
	syd = newAirport("SYD", "Sydney", "AU", "Australia/Sydney")
)

func newAirport(code, city, country, zone string) flight.Airport {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		panic(fmt.Sprintf("load zone %s: %v", zone, err))
	}
	return flight.Airport{Code: code, Name: city + " " + code, City: city, Country: country, Location: loc}
}

// localLayout is how tests write times: the wall clock at one airport.
const localLayout = "2006-01-02 15:04"

// local returns the instant at which the clocks at airport a read value.
func local(a flight.Airport, value string) time.Time {
	t, err := time.ParseInLocation(localLayout, value, a.Location)
	if err != nil {
		panic(fmt.Sprintf("parse %q: %v", value, err))
	}
	return t
}

// newFlight builds a flight the way the dataset loader does: departure and
// arrival are local times, each read in its own airport's zone. price is in
// whole currency units.
func newFlight(number string, from, to flight.Airport, departs, arrives string, price int64) flight.Flight {
	return flight.Flight{
		Number:      number,
		Airline:     "SkyPath",
		Origin:      from,
		Destination: to,
		Departure:   local(from, departs),
		Arrival:     local(to, arrives),
		Price:       flight.Money(price * 100),
	}
}

// testAirports is every airport the hand-written tests know about.
var testAirports = []flight.Airport{jfk, lga, bos, mia, ord, den, phx, lax, sfo, sea, lhr, cdg, mex, nrt, syd}

// memSource is an in-memory FlightSource. Like the real store it keeps each
// airport's departures sorted by time and answers "at or after" with a
// binary search, so benchmarks measure the engine rather than a linear scan.
type memSource struct {
	airports   map[string]flight.Airport
	departures map[string][]flight.Flight
}

// newSource indexes flights for the given airports. Flights that depart at
// the same instant keep the order they were given in, so tests can check
// that results do not depend on insertion order.
func newSource(airports []flight.Airport, flights []flight.Flight) *memSource {
	s := &memSource{
		airports:   make(map[string]flight.Airport, len(airports)),
		departures: make(map[string][]flight.Flight),
	}
	for _, a := range airports {
		s.airports[a.Code] = a
	}
	for _, f := range flights {
		s.departures[f.Origin.Code] = append(s.departures[f.Origin.Code], f)
	}
	for _, list := range s.departures {
		slices.SortStableFunc(list, func(a, b flight.Flight) int {
			return a.Departure.Compare(b.Departure)
		})
	}
	return s
}

// sourceOf indexes flights over testAirports.
func sourceOf(flights ...flight.Flight) *memSource {
	return newSource(testAirports, flights)
}

func (s *memSource) Airport(code string) (flight.Airport, bool) {
	a, ok := s.airports[code]
	return a, ok
}

func (s *memSource) DeparturesFrom(code string, notBefore time.Time) []flight.Flight {
	list := s.departures[code]
	i, _ := slices.BinarySearchFunc(list, notBefore, func(f flight.Flight, t time.Time) int {
		return f.Departure.Compare(t)
	})
	// Clipping stops a caller that appends to the result from overwriting
	// the index.
	return slices.Clip(list[i:])
}
