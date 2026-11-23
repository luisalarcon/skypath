package flight

import (
	"testing"
	"time"

	// Tests resolve real time zones; embedding the database keeps them
	// independent of what the host has installed.
	_ "time/tzdata"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load zone %s: %v", name, err)
	}
	return loc
}

func mustLocal(t *testing.T, value string, loc *time.Location) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02T15:04:05", value, loc)
	if err != nil {
		t.Fatalf("parse %s: %v", value, err)
	}
	return parsed
}

func TestNormalizeAndValidCode(t *testing.T) {
	if got := NormalizeCode("  jfk "); got != "JFK" {
		t.Errorf("NormalizeCode = %q, want JFK", got)
	}
	for _, code := range []string{"JFK", "LAX", "XXX"} {
		if !ValidCode(code) {
			t.Errorf("ValidCode(%q) = false, want true", code)
		}
	}
	for _, code := range []string{"", "JF", "JFKK", "jfk", "J1K", "J K", "ÄBC"} {
		if ValidCode(code) {
			t.Errorf("ValidCode(%q) = true, want false", code)
		}
	}
}

func TestFlightDurationAcrossTimeZones(t *testing.T) {
	newYork := mustZone(t, "America/New_York")
	losAngeles := mustZone(t, "America/Los_Angeles")
	sydney := mustZone(t, "Australia/Sydney")

	// Local clocks read 08:30 to 11:45, but the flight is 6h15m.
	westbound := Flight{
		Origin:      Airport{Code: "JFK", Country: "US", Location: newYork},
		Destination: Airport{Code: "LAX", Country: "US", Location: losAngeles},
		Departure:   mustLocal(t, "2024-03-15T08:30:00", newYork),
		Arrival:     mustLocal(t, "2024-03-15T11:45:00", losAngeles),
	}
	if got, want := westbound.Duration(), 6*time.Hour+15*time.Minute; got != want {
		t.Errorf("JFK-LAX duration = %s, want %s", got, want)
	}
	if !westbound.Domestic() {
		t.Errorf("JFK-LAX should be domestic")
	}

	// Crossing the date line: lands at an earlier local time on the same
	// calendar date, yet takes 15 hours.
	dateLine := Flight{
		Origin:      Airport{Code: "SYD", Country: "AU", Location: sydney},
		Destination: Airport{Code: "LAX", Country: "US", Location: losAngeles},
		Departure:   mustLocal(t, "2024-03-15T09:00:00", sydney),
		Arrival:     mustLocal(t, "2024-03-15T06:00:00", losAngeles),
	}
	if got, want := dateLine.Duration(), 15*time.Hour; got != want {
		t.Errorf("SYD-LAX duration = %s, want %s", got, want)
	}
	if dateLine.Domestic() {
		t.Errorf("SYD-LAX should be international")
	}
}

func TestItineraryTotals(t *testing.T) {
	newYork := mustZone(t, "America/New_York")
	chicago := mustZone(t, "America/Chicago")
	losAngeles := mustZone(t, "America/Los_Angeles")

	jfk := Airport{Code: "JFK", Country: "US", Location: newYork}
	ord := Airport{Code: "ORD", Country: "US", Location: chicago}
	lax := Airport{Code: "LAX", Country: "US", Location: losAngeles}

	first := Flight{
		Number: "SP110", Origin: jfk, Destination: ord,
		Departure: mustLocal(t, "2024-03-15T07:00:00", newYork),
		Arrival:   mustLocal(t, "2024-03-15T08:30:00", chicago),
		Price:     18900,
	}
	second := Flight{
		Number: "SP120", Origin: ord, Destination: lax,
		Departure: mustLocal(t, "2024-03-15T09:15:00", chicago),
		Arrival:   mustLocal(t, "2024-03-15T11:30:00", losAngeles),
		Price:     22500,
	}
	itinerary := Itinerary{
		Segments: []Flight{first, second},
		Layovers: []Layover{{Airport: ord, Duration: second.Departure.Sub(first.Arrival), Kind: Domestic}},
	}

	if got := itinerary.Stops(); got != 1 {
		t.Errorf("Stops = %d, want 1", got)
	}
	if got, want := itinerary.Duration(), 7*time.Hour+30*time.Minute; got != want {
		t.Errorf("Duration = %s, want %s", got, want)
	}
	if got, want := itinerary.Price(), Money(41400); got != want {
		t.Errorf("Price = %d, want %d", got, want)
	}
	if got, want := itinerary.Layovers[0].Duration, 45*time.Minute; got != want {
		t.Errorf("layover = %s, want %s", got, want)
	}
}

func TestEmptyItinerary(t *testing.T) {
	var empty Itinerary
	if empty.Stops() != 0 || empty.Duration() != 0 || empty.Price() != 0 {
		t.Errorf("empty itinerary should report zero stops, duration and price")
	}
	if !empty.Departure().IsZero() || !empty.Arrival().IsZero() {
		t.Errorf("empty itinerary should report zero times")
	}
}
