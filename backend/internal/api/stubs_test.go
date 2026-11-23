package api

import (
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
