package store

import (
	"slices"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"

	// Tests resolve real time zones; embedding the database keeps them
	// independent of what the host has installed.
	_ "time/tzdata"
)

func mustZone(t testing.TB, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load zone %s: %v", name, err)
	}
	return loc
}

// testAirports returns a small network, deliberately not in code order.
func testAirports(t testing.TB) []flight.Airport {
	t.Helper()
	return []flight.Airport{
		{Code: "LAX", Name: "Los Angeles International", City: "Los Angeles", Country: "US", Location: mustZone(t, "America/Los_Angeles")},
		{Code: "JFK", Name: "John F. Kennedy International", City: "New York", Country: "US", Location: mustZone(t, "America/New_York")},
		{Code: "ORD", Name: "O'Hare International", City: "Chicago", Country: "US", Location: mustZone(t, "America/Chicago")},
		{Code: "NRT", Name: "Narita International", City: "Tokyo", Country: "JP", Location: mustZone(t, "Asia/Tokyo")},
	}
}

func codes(airports []flight.Airport) []string {
	out := make([]string, len(airports))
	for i, a := range airports {
		out[i] = a.Code
	}
	return out
}

func TestAirportLookup(t *testing.T) {
	s := New(testAirports(t), nil)

	got, ok := s.Airport("JFK")
	if !ok {
		t.Fatalf("Airport(JFK) not found")
	}
	if got.Name != "John F. Kennedy International" || got.Location.String() != "America/New_York" {
		t.Errorf("Airport(JFK) = %+v", got)
	}

	for _, code := range []string{"XXX", "", "jfk", " JFK"} {
		if _, ok := s.Airport(code); ok {
			t.Errorf("Airport(%q) found, want miss", code)
		}
	}
}

func TestAirportsOrderedByCode(t *testing.T) {
	s := New(testAirports(t), nil)
	if got, want := codes(s.Airports()), []string{"JFK", "LAX", "NRT", "ORD"}; !slices.Equal(got, want) {
		t.Errorf("Airports = %v, want %v", got, want)
	}
}

func TestFirstAirportWithACodeWins(t *testing.T) {
	airports := testAirports(t)
	duplicate := airports[1]
	duplicate.Name = "Second JFK"
	s := New(append(airports, duplicate), nil)

	if got, _ := s.Airport("JFK"); got.Name != "John F. Kennedy International" {
		t.Errorf("Airport(JFK).Name = %q, want the first record", got.Name)
	}
	if got := len(s.Airports()); got != 4 {
		t.Errorf("len(Airports) = %d, want 4", got)
	}
}

func TestAirportsResultCannotChangeTheStore(t *testing.T) {
	s := New(testAirports(t), nil)
	list := s.Airports()
	list[0].Name = "overwritten"
	list[1], list[2] = list[2], list[1]

	if got := s.Airports(); got[0].Name != "John F. Kennedy International" || got[1].Code != "LAX" {
		t.Errorf("store changed through the Airports result: %v", codes(got))
	}
}

func TestNewCopiesAirportInput(t *testing.T) {
	airports := testAirports(t)
	s := New(airports, nil)

	airports[1].Name = "overwritten"
	airports[0] = flight.Airport{Code: "ZZZ"}

	if got, ok := s.Airport("JFK"); !ok || got.Name != "John F. Kennedy International" {
		t.Errorf("Airport(JFK) = %+v, %v after caller modified its slice", got, ok)
	}
	if _, ok := s.Airport("ZZZ"); ok {
		t.Errorf("Airport(ZZZ) found after caller modified its slice")
	}
}

func TestZeroStoreIsEmpty(t *testing.T) {
	var s Store
	if _, ok := s.Airport("JFK"); ok {
		t.Errorf("zero Store found an airport")
	}
	if len(s.Airports()) != 0 || s.Len() != 0 {
		t.Errorf("zero Store is not empty")
	}
}
