package search

import (
	"fmt"
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
