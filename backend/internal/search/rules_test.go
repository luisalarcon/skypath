package search

import (
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

func TestConnectionKind(t *testing.T) {
	cases := []struct {
		name          string
		first, second flight.Flight
		want          flight.ConnectionKind
	}{
		{
			name:   "all three airports in one country",
			first:  newFlight("SP1", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 100),
			second: newFlight("SP2", ord, lax, "2024-03-15 10:00", "2024-03-15 12:15", 100),
			want:   flight.Domestic,
		},
		{
			name:   "neither flight domestic",
			first:  newFlight("SP1", jfk, lhr, "2024-03-15 18:00", "2024-03-16 06:00", 100),
			second: newFlight("SP2", lhr, cdg, "2024-03-16 08:00", "2024-03-16 10:15", 100),
			want:   flight.International,
		},
		{
			name:   "domestic flight feeding an international one",
			first:  newFlight("SP1", mia, jfk, "2024-03-15 07:00", "2024-03-15 10:00", 100),
			second: newFlight("SP2", jfk, mex, "2024-03-15 12:00", "2024-03-15 15:30", 100),
			want:   flight.International,
		},
		{
			name:   "international flight feeding a domestic one",
			first:  newFlight("SP1", lhr, jfk, "2024-03-15 09:00", "2024-03-15 12:00", 100),
			second: newFlight("SP2", jfk, sfo, "2024-03-15 14:00", "2024-03-15 17:30", 100),
			want:   flight.International,
		},
		{
			name:   "out and back into the same foreign country",
			first:  newFlight("SP1", lhr, jfk, "2024-03-15 09:00", "2024-03-15 12:00", 100),
			second: newFlight("SP2", jfk, lhr, "2024-03-15 18:00", "2024-03-16 06:00", 100),
			want:   flight.International,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := connectionKind(tc.first, tc.second); got != tc.want {
				t.Errorf("connectionKind(%s %s-%s, %s %s-%s) = %s, want %s",
					tc.first.Number, tc.first.Origin.Code, tc.first.Destination.Code,
					tc.second.Number, tc.second.Origin.Code, tc.second.Destination.Code,
					got, tc.want)
			}
		})
	}
}

// connectingPair returns a flight from -> via that lands at via at the local
// time arrives, and a flight via -> to that leaves via at the local time
// departs. Each flight takes two hours.
func connectingPair(from, via, to flight.Airport, arrives, departs string) (flight.Flight, flight.Flight) {
	const block = 2 * time.Hour
	arrival := local(via, arrives)
	departure := local(via, departs)
	first := flight.Flight{
		Number: "SP1", Origin: from, Destination: via,
		Departure: arrival.Add(-block).In(from.Location), Arrival: arrival,
	}
	second := flight.Flight{
		Number: "SP2", Origin: via, Destination: to,
		Departure: departure, Arrival: departure.Add(block).In(to.Location),
	}
	return first, second
}

func TestConnectLayoverBounds(t *testing.T) {
	const arrives = "2024-03-15 10:00"
	cases := []struct {
		name          string
		from, via, to flight.Airport
		departs       string
		wantOK        bool
		wantKind      flight.ConnectionKind
		wantDuration  time.Duration
	}{
		{"domestic 44 minutes", jfk, ord, lax, "2024-03-15 10:44", false, flight.Domestic, 44 * time.Minute},
		{"domestic 45 minutes", jfk, ord, lax, "2024-03-15 10:45", true, flight.Domestic, 45 * time.Minute},
		{"domestic 46 minutes", jfk, ord, lax, "2024-03-15 10:46", true, flight.Domestic, 46 * time.Minute},
		{"domestic 360 minutes", jfk, ord, lax, "2024-03-15 16:00", true, flight.Domestic, 360 * time.Minute},
		{"domestic 361 minutes", jfk, ord, lax, "2024-03-15 16:01", false, flight.Domestic, 361 * time.Minute},

		{"international 89 minutes", jfk, lhr, cdg, "2024-03-15 11:29", false, flight.International, 89 * time.Minute},
		{"international 90 minutes", jfk, lhr, cdg, "2024-03-15 11:30", true, flight.International, 90 * time.Minute},
		{"international 91 minutes", jfk, lhr, cdg, "2024-03-15 11:31", true, flight.International, 91 * time.Minute},
		{"international 360 minutes", jfk, lhr, cdg, "2024-03-15 16:00", true, flight.International, 360 * time.Minute},
		{"international 361 minutes", jfk, lhr, cdg, "2024-03-15 16:01", false, flight.International, 361 * time.Minute},

		// Long enough for a domestic connection, too short for an
		// international one: the mixed pair must be held to 90 minutes.
		{"domestic then international at 60 minutes", mia, jfk, mex, "2024-03-15 11:00", false, flight.International, 60 * time.Minute},
		{"international then domestic at 60 minutes", lhr, jfk, sfo, "2024-03-15 11:00", false, flight.International, 60 * time.Minute},
		{"domestic then international at 90 minutes", mia, jfk, mex, "2024-03-15 11:30", true, flight.International, 90 * time.Minute},
		{"international then domestic at 90 minutes", lhr, jfk, sfo, "2024-03-15 11:30", true, flight.International, 90 * time.Minute},

		{"departs before the arrival", jfk, ord, lax, "2024-03-15 09:30", false, flight.Domestic, -30 * time.Minute},
		{"departs at the arrival instant", jfk, ord, lax, "2024-03-15 10:00", false, flight.Domestic, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, second := connectingPair(tc.from, tc.via, tc.to, arrives, tc.departs)
			if got := connectionKind(first, second); got != tc.wantKind {
				t.Fatalf("connectionKind = %s, want %s", got, tc.wantKind)
			}
			if got := second.Departure.Sub(first.Arrival); got != tc.wantDuration {
				t.Fatalf("fixture wait = %s, want %s", got, tc.wantDuration)
			}

			layover, ok := connect(first, second)
			if ok != tc.wantOK {
				t.Fatalf("connect ok = %t, want %t", ok, tc.wantOK)
			}
			if !ok {
				if layover != (flight.Layover{}) {
					t.Errorf("rejected connection returned layover %+v, want zero value", layover)
				}
				return
			}
			want := flight.Layover{Airport: tc.via, Duration: tc.wantDuration, Kind: tc.wantKind}
			if layover != want {
				t.Errorf("layover = %+v, want %+v", layover, want)
			}
		})
	}
}

func TestConnectMeasuresTheWaitBetweenInstants(t *testing.T) {
	// Phoenix keeps standard time all year, so in March its clocks match Los
	// Angeles although it is a mountain-time city: the 06:00 to 06:30 flight
	// really is 30 minutes. The onward leg runs on Denver's daylight time.
	// Every figure must come from instants, never from clock faces.
	first := newFlight("SP1", lax, phx, "2024-03-15 06:00", "2024-03-15 06:30", 100)
	second := newFlight("SP2", phx, den, "2024-03-15 07:15", "2024-03-15 08:45", 100)
	if got, want := first.Duration(), 30*time.Minute; got != want {
		t.Fatalf("LAX-PHX duration = %s, want %s", got, want)
	}
	layover, ok := connect(first, second)
	if !ok {
		t.Fatalf("connect rejected a 45-minute domestic connection at PHX")
	}
	if layover.Duration != 45*time.Minute || layover.Airport.Code != "PHX" || layover.Kind != flight.Domestic {
		t.Errorf("layover = %+v, want 45m domestic at PHX", layover)
	}
}

func TestConnectRequiresTheSameAirport(t *testing.T) {
	// LGA and JFK both serve New York, but a passenger landing at one cannot
	// connect at the other, however comfortable the gap.
	arriveLGA := newFlight("SP1", ord, lga, "2024-03-15 07:00", "2024-03-15 10:00", 100)
	for _, departs := range []string{"2024-03-15 11:00", "2024-03-15 12:00", "2024-03-15 14:00"} {
		leaveJFK := newFlight("SP2", jfk, lhr, departs, "2024-03-16 06:00", 100)
		if layover, ok := connect(arriveLGA, leaveJFK); ok {
			t.Errorf("LGA arrival connected to JFK departure at %s: %+v", departs, layover)
		}
		leaveJFKDomestic := newFlight("SP3", jfk, bos, departs, "2024-03-15 16:00", 100)
		if layover, ok := connect(arriveLGA, leaveJFKDomestic); ok {
			t.Errorf("LGA arrival connected to domestic JFK departure at %s: %+v", departs, layover)
		}
	}
}
