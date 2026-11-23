package api

import (
	"context"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"

	// The fixtures resolve real IANA zones; embedding the database keeps the
	// tests independent of what the host has installed.
	_ "time/tzdata"
)

// airports holds fixture airports in their real time zones.
type airports struct {
	JFK, LAX, ORD, SFO, DEN, NRT flight.Airport
}

func newAirports(t *testing.T) airports {
	t.Helper()
	zone := func(name string) *time.Location {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("load zone %s: %v", name, err)
		}
		return loc
	}
	return airports{
		JFK: flight.Airport{Code: "JFK", Name: "John F. Kennedy International", City: "New York", Country: "US", Location: zone("America/New_York")},
		LAX: flight.Airport{Code: "LAX", Name: "Los Angeles International", City: "Los Angeles", Country: "US", Location: zone("America/Los_Angeles")},
		ORD: flight.Airport{Code: "ORD", Name: "O'Hare International", City: "Chicago", Country: "US", Location: zone("America/Chicago")},
		SFO: flight.Airport{Code: "SFO", Name: "San Francisco International", City: "San Francisco", Country: "US", Location: zone("America/Los_Angeles")},
		DEN: flight.Airport{Code: "DEN", Name: "Denver International", City: "Denver", Country: "US", Location: zone("America/Denver")},
		NRT: flight.Airport{Code: "NRT", Name: "Narita International", City: "Tokyo", Country: "JP", Location: zone("Asia/Tokyo")},
	}
}

// stubDirectory is an AirportDirectory over a fixed, ordered list.
type stubDirectory []flight.Airport

func (d stubDirectory) Airport(code string) (flight.Airport, bool) {
	for _, a := range d {
		if a.Code == code {
			return a, true
		}
	}
	return flight.Airport{}, false
}

func (d stubDirectory) Airports() []flight.Airport {
	return d
}

// local returns the wall-clock time "2006-01-02T15:04:05" in the airport's
// own zone, as the flight data does.
func local(t *testing.T, a flight.Airport, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02T15:04:05", value, a.Location)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

// leg builds a flight of the fixture airline between two airports, given
// local departure and arrival times and a price in cents.
func leg(t *testing.T, number, aircraft string, from flight.Airport, dep string, to flight.Airport, arr string, price flight.Money) flight.Flight {
	t.Helper()
	return flight.Flight{
		Number:      number,
		Airline:     "SkyPath Airways",
		Aircraft:    aircraft,
		Origin:      from,
		Destination: to,
		Departure:   local(t, from, dep),
		Arrival:     local(t, to, arr),
		Price:       price,
	}
}

// itineraryOf chains flights into an itinerary, deriving each layover from
// the gap between consecutive flights.
func itineraryOf(legs ...flight.Flight) flight.Itinerary {
	it := flight.Itinerary{Segments: legs}
	for i := 0; i+1 < len(legs); i++ {
		kind := flight.International
		if legs[i].Domestic() && legs[i+1].Domestic() {
			kind = flight.Domestic
		}
		it.Layovers = append(it.Layovers, flight.Layover{
			Airport:  legs[i].Destination,
			Duration: legs[i+1].Departure.Sub(legs[i].Arrival),
			Kind:     kind,
		})
	}
	return it
}

// directJFKLAX is a direct itinerary; the clocks read 08:30 to 11:45 but the
// flight takes 375 minutes.
func directJFKLAX(t *testing.T, a airports) flight.Itinerary {
	return itineraryOf(leg(t, "SP101", "A320", a.JFK, "2024-03-15T08:30:00", a.LAX, "2024-03-15T11:45:00", 29900))
}

// oneStopJFKLAX connects at ORD with the shortest legal domestic layover.
func oneStopJFKLAX(t *testing.T, a airports) flight.Itinerary {
	return itineraryOf(
		leg(t, "SP110", "A319", a.JFK, "2024-03-15T07:00:00", a.ORD, "2024-03-15T08:30:00", 18900),
		leg(t, "SP120", "A320", a.ORD, "2024-03-15T09:15:00", a.LAX, "2024-03-15T11:30:00", 22500),
	)
}

// twoStopSFONRT crosses the date line: it leaves on the 15th and lands on
// the 17th, with a domestic and then an international layover.
func twoStopSFONRT(t *testing.T, a airports) flight.Itinerary {
	return itineraryOf(
		leg(t, "SP981", "B737", a.SFO, "2024-03-15T14:00:00", a.DEN, "2024-03-15T17:30:00", 19900),
		leg(t, "SP212", "A320", a.DEN, "2024-03-15T19:00:00", a.LAX, "2024-03-15T20:15:00", 13900),
		leg(t, "SP501", "B777", a.LAX, "2024-03-15T23:00:00", a.NRT, "2024-03-17T03:00:00", 79900),
	)
}

// searcherFunc adapts a function to the Searcher interface.
type searcherFunc func(ctx context.Context, q flight.Query) ([]flight.Itinerary, error)

func (f searcherFunc) Search(ctx context.Context, q flight.Query) ([]flight.Itinerary, error) {
	return f(ctx, q)
}

// sampleDataset matches api/examples/dataset.json and ready.json.
func sampleDataset() DatasetInfo {
	return DatasetInfo{
		Airports:       25,
		Flights:        302,
		FirstDeparture: flight.Date{Year: 2024, Month: time.March, Day: 15},
		LastDeparture:  flight.Date{Year: 2024, Month: time.March, Day: 16},
		Skipped:        []SkippedRecord{{Record: "flight SP995", Reason: `unknown origin airport "JKF"`}},
	}
}
