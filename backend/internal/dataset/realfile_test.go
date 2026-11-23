package dataset

import (
	"strings"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// realSchedule is the schedule the service ships with, at the repository
// root.
const realSchedule = "../../../flights.json"

func loadReal(t *testing.T) *Dataset {
	t.Helper()
	d, err := Load(realSchedule)
	if err != nil {
		t.Fatalf("Load(%s): %v", realSchedule, err)
	}
	return d
}

func TestRealScheduleCounts(t *testing.T) {
	d := loadReal(t)

	if got := len(d.Airports); got != 25 {
		t.Errorf("airports = %d, want 25", got)
	}
	if got := len(d.Flights); got != 302 {
		t.Errorf("flights = %d, want 302", got)
	}
	if len(d.Skipped) != 1 {
		t.Fatalf("skipped = %q, want exactly one record", d.Skipped)
	}
	skipped := d.Skipped[0]
	if skipped.Record != "flight SP995" || !strings.Contains(skipped.Reason, "JKF") {
		t.Errorf("skipped = %q, want flight SP995 with a reason naming JKF", skipped)
	}
	t.Logf("skipped: %s", skipped)
}

// TestRealScheduleProfile checks the description of the file in
// docs/data.md, so the documentation and the loader cannot drift apart.
func TestRealScheduleProfile(t *testing.T) {
	d := loadReal(t)

	countries := make(map[string]int)
	for _, a := range d.Airports {
		countries[a.Country]++
	}
	if len(countries) != 12 || countries["US"] != 13 {
		t.Errorf("countries = %d with %d US airports, want 12 with 13", len(countries), countries["US"])
	}

	routes := make(map[[2]string]bool)
	departureDays := make(map[flight.Date]int)
	var lastArrival flight.Date
	for _, f := range d.Flights {
		routes[[2]string{f.Origin.Code, f.Destination.Code}] = true
		departureDays[flight.DateOf(f.Departure)]++
		if day := flight.DateOf(f.Arrival); day.Compare(lastArrival) > 0 {
			lastArrival = day
		}
	}
	if got := len(routes); got != 132 {
		t.Errorf("distinct routes = %d, want 132", got)
	}

	// The documented 301 departures on the 15th count every record; one of
	// them is the skipped SP995, which leaves 300 valid flights that day.
	march := func(day int) flight.Date { return flight.Date{Year: 2024, Month: time.March, Day: day} }
	if departureDays[march(15)] != 300 || departureDays[march(16)] != 2 || len(departureDays) != 2 {
		t.Errorf("valid departures by local date = %v, want 300 on the 15th and 2 on the 16th", departureDays)
	}
	if lastArrival != march(17) {
		t.Errorf("last local arrival date = %s, want 2024-03-17", lastArrival)
	}
}

func TestRealScheduleSpotChecks(t *testing.T) {
	d := loadReal(t)
	byNumber := make(map[string]flight.Flight, len(d.Flights))
	for _, f := range d.Flights {
		byNumber[f.Number] = f
	}

	cases := []struct {
		number      string
		origin      string
		destination string
		minutes     int    // 0 when not checked
		price       string // "" when not checked
	}{
		{"SP101", "JFK", "LAX", 375, "299.00"},
		{"SP996", "LAX", "SFO", 0, "99.00"},  // price given as the string "99"
		{"SP997", "LAX", "SFO", 0, "109.00"}, // price given as the integer 109
		{"SP998", "SFO", "LAX", 0, "95.00"},  // price given as the string "95.00"
		{"SP540", "SYD", "LAX", 900, ""},     // crosses the date line
		{"SP300", "PHX", "LAX", 30, ""},      // Phoenix keeps standard time all year
	}
	for _, tc := range cases {
		t.Run(tc.number, func(t *testing.T) {
			f, ok := byNumber[tc.number]
			if !ok {
				t.Fatalf("flight %s not loaded", tc.number)
			}
			if f.Origin.Code != tc.origin || f.Destination.Code != tc.destination {
				t.Errorf("route = %s to %s, want %s to %s", f.Origin.Code, f.Destination.Code, tc.origin, tc.destination)
			}
			if tc.minutes != 0 {
				if got, want := f.Duration(), time.Duration(tc.minutes)*time.Minute; got != want {
					t.Errorf("duration = %s, want %s", got, want)
				}
			}
			if tc.price != "" {
				if got := f.Price.String(); got != tc.price {
					t.Errorf("price = %s, want %s", got, tc.price)
				}
			}
		})
	}
}
