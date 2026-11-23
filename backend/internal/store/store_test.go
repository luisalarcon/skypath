package store

import (
	"fmt"
	"slices"
	"sync"
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

// testSchedule is a set of departures from JFK and ORD, deliberately out of
// order. SP103 and SP102 leave JFK at the same instant to exercise the
// flight-number tie-break.
func testSchedule(t testing.TB) ([]flight.Airport, []flight.Flight) {
	t.Helper()
	airports := testAirports(t)
	jfk, lax, ord, nrt := airports[1], airports[0], airports[2], airports[3]
	local := func(value string, loc *time.Location) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02T15:04:05", value, loc)
		if err != nil {
			t.Fatalf("parse %s: %v", value, err)
		}
		return parsed
	}
	leg := func(number string, from, to flight.Airport, departure, arrival string) flight.Flight {
		return flight.Flight{
			Number:      number,
			Origin:      from,
			Destination: to,
			Departure:   local(departure, from.Location),
			Arrival:     local(arrival, to.Location),
			Price:       10000,
		}
	}
	flights := []flight.Flight{
		leg("SP104", jfk, lax, "2024-03-15T18:00:00", "2024-03-15T21:15:00"),
		leg("SP103", jfk, ord, "2024-03-15T12:00:00", "2024-03-15T13:30:00"),
		leg("SP101", jfk, lax, "2024-03-15T08:30:00", "2024-03-15T11:45:00"),
		leg("SP200", ord, lax, "2024-03-15T09:00:00", "2024-03-15T11:15:00"),
		leg("SP102", jfk, nrt, "2024-03-15T12:00:00", "2024-03-16T15:00:00"),
	}
	return airports, flights
}

func numbers(flights []flight.Flight) []string {
	out := make([]string, len(flights))
	for i, f := range flights {
		out[i] = f.Number
	}
	return out
}

func TestLen(t *testing.T) {
	s := New(testSchedule(t))
	if got := s.Len(); got != 5 {
		t.Errorf("Len = %d, want 5", got)
	}
}

func TestDeparturesFrom(t *testing.T) {
	airports, flights := testSchedule(t)
	s := New(airports, flights)
	newYork := airports[1].Location
	at := func(value string) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02T15:04:05", value, newYork)
		if err != nil {
			t.Fatalf("parse %s: %v", value, err)
		}
		return parsed
	}

	cases := []struct {
		name      string
		code      string
		notBefore time.Time
		want      []string
	}{
		{"zero time returns every departure in order", "JFK", time.Time{}, []string{"SP101", "SP102", "SP103", "SP104"}},
		{"before the first departure", "JFK", at("2024-03-15T00:00:00"), []string{"SP101", "SP102", "SP103", "SP104"}},
		{"exactly at a departure includes it", "JFK", at("2024-03-15T08:30:00"), []string{"SP101", "SP102", "SP103", "SP104"}},
		{"one second after a departure excludes it", "JFK", at("2024-03-15T08:30:01"), []string{"SP102", "SP103", "SP104"}},
		{"shared instant keeps both, by flight number", "JFK", at("2024-03-15T12:00:00"), []string{"SP102", "SP103", "SP104"}},
		{"exactly at the last departure", "JFK", at("2024-03-15T18:00:00"), []string{"SP104"}},
		{"after the last departure", "JFK", at("2024-03-15T18:00:01"), []string{}},
		{"same instant expressed in another zone", "JFK", at("2024-03-15T08:30:00").UTC(), []string{"SP101", "SP102", "SP103", "SP104"}},
		{"other origin", "ORD", time.Time{}, []string{"SP200"}},
		{"airport with no departures", "LAX", time.Time{}, []string{}},
		{"unknown airport", "XXX", time.Time{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.DeparturesFrom(tc.code, tc.notBefore)
			if !slices.Equal(numbers(got), tc.want) {
				t.Errorf("DeparturesFrom(%s, %s) = %v, want %v", tc.code, tc.notBefore, numbers(got), tc.want)
			}
		})
	}
}

func TestDeparturesOrderDoesNotDependOnInputOrder(t *testing.T) {
	airports, flights := testSchedule(t)
	forward := numbers(New(airports, flights).DeparturesFrom("JFK", time.Time{}))
	slices.Reverse(flights)
	backward := numbers(New(airports, flights).DeparturesFrom("JFK", time.Time{}))
	if !slices.Equal(forward, backward) {
		t.Errorf("order depends on input: %v vs %v", forward, backward)
	}
}

func TestAppendToDeparturesCannotCorruptTheStore(t *testing.T) {
	// Three departures from JFK: a list built by append then has room for a
	// fourth, which is exactly the room an unclipped window would expose.
	airports, flights := testSchedule(t)
	var three []flight.Flight
	for _, f := range flights {
		if f.Origin.Code == "JFK" && len(three) < 3 {
			three = append(three, f)
		}
	}
	s := New(airports, three)
	want := numbers(s.DeparturesFrom("JFK", time.Time{}))

	first := s.DeparturesFrom("JFK", time.Time{})
	if cap(first) != len(first) {
		t.Errorf("cap = %d, want clipped to len %d", cap(first), len(first))
	}
	second := s.DeparturesFrom("JFK", first[1].Departure)

	// Two requests extending their own results must not see each other's
	// additions, which would happen if both appends wrote the same slot.
	mine := append(first, flight.Flight{Number: "MINE"})
	theirs := append(second, flight.Flight{Number: "THEIRS"})
	if got := mine[len(mine)-1].Number; got != "MINE" {
		t.Errorf("first caller's appended flight = %s, want MINE", got)
	}
	if got := theirs[len(theirs)-1].Number; got != "THEIRS" {
		t.Errorf("second caller's appended flight = %s, want THEIRS", got)
	}
	if got := numbers(s.DeparturesFrom("JFK", time.Time{})); !slices.Equal(got, want) {
		t.Errorf("after append, DeparturesFrom = %v, want %v", got, want)
	}
}

func TestNewCopiesFlightInput(t *testing.T) {
	airports, flights := testSchedule(t)
	s := New(airports, flights)

	flights[2].Number = "overwritten"
	flights[0] = flight.Flight{Number: "SP999", Origin: airports[1]}
	slices.Reverse(flights)

	if got, want := numbers(s.DeparturesFrom("JFK", time.Time{})), []string{"SP101", "SP102", "SP103", "SP104"}; !slices.Equal(got, want) {
		t.Errorf("after caller modified its slice, DeparturesFrom = %v, want %v", got, want)
	}
}

func TestDeparturesFromAllocatesNothing(t *testing.T) {
	airports, flights := testSchedule(t)
	s := New(airports, flights)
	notBefore := flights[1].Departure

	allocs := testing.AllocsPerRun(100, func() {
		_ = s.DeparturesFrom("JFK", notBefore)
		_ = s.DeparturesFrom("XXX", notBefore)
	})
	if allocs != 0 {
		t.Errorf("DeparturesFrom allocates %.1f times per call, want 0", allocs)
	}
}

func TestConcurrentReaders(t *testing.T) {
	s := New(testSchedule(t))
	want := []string{"SP101", "SP102", "SP103", "SP104"}

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 200 {
				if _, ok := s.Airport("JFK"); !ok {
					t.Error("Airport(JFK) not found")
					return
				}
				if got := len(s.Airports()); got != 4 {
					t.Errorf("len(Airports) = %d, want 4", got)
					return
				}
				if got := numbers(s.DeparturesFrom("JFK", time.Time{})); !slices.Equal(got, want) {
					t.Errorf("DeparturesFrom = %v, want %v", got, want)
					return
				}
			}
		})
	}
	wg.Wait()
}

func BenchmarkDeparturesFrom(b *testing.B) {
	airports := testAirports(b)
	jfk, lax := airports[1], airports[0]
	start := time.Date(2024, time.March, 15, 0, 0, 0, 0, jfk.Location)

	// One busy airport: a departure every five minutes for a day.
	const n = 288
	flights := make([]flight.Flight, n)
	for i := range flights {
		departure := start.Add(time.Duration(i) * 5 * time.Minute)
		flights[i] = flight.Flight{
			Number:      fmt.Sprintf("SP%04d", i),
			Origin:      jfk,
			Destination: lax,
			Departure:   departure,
			Arrival:     departure.Add(6 * time.Hour),
		}
	}
	s := New(airports, flights)
	noon := start.Add(12 * time.Hour)

	b.ReportAllocs()
	for b.Loop() {
		if got := s.DeparturesFrom("JFK", noon); len(got) != n/2 {
			b.Fatalf("len = %d, want %d", len(got), n/2)
		}
	}
}
