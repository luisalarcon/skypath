package search

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// searchDate is the date most tests search on.
var searchDate = flight.Date{Year: 2024, Month: time.March, Day: 15}

// mustSearch runs a search that is expected to succeed and checks that every
// itinerary it returns obeys the rules, whatever the test is looking at.
func mustSearch(t *testing.T, src FlightSource, q flight.Query) []flight.Itinerary {
	t.Helper()
	got, err := NewEngine(src).Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search(%s-%s on %s) returned error: %v", q.Origin, q.Destination, q.Date, err)
	}
	if got == nil {
		t.Fatalf("Search(%s-%s on %s) returned a nil slice, want non-nil", q.Origin, q.Destination, q.Date)
	}
	for _, it := range got {
		checkItinerary(t, src, q, it)
	}
	return got
}

func query(origin, destination flight.Airport, date flight.Date) flight.Query {
	return flight.Query{Origin: origin.Code, Destination: destination.Code, Date: date}
}

// checkItinerary verifies the invariants every returned itinerary must hold,
// independently of how the engine found it.
func checkItinerary(t *testing.T, src FlightSource, q flight.Query, it flight.Itinerary) {
	t.Helper()
	name := key(it)
	segments := it.Segments

	if len(segments) < 1 || len(segments) > maxFlights {
		t.Errorf("%s: %d segments, want 1 to %d", name, len(segments), maxFlights)
		return
	}
	if len(it.Layovers) != len(segments)-1 {
		t.Errorf("%s: %d layovers for %d segments", name, len(it.Layovers), len(segments))
		return
	}
	if segments[0].Origin.Code != q.Origin || segments[len(segments)-1].Destination.Code != q.Destination {
		t.Errorf("%s: runs %s-%s, want %s-%s", name,
			segments[0].Origin.Code, segments[len(segments)-1].Destination.Code, q.Origin, q.Destination)
	}

	origin, _ := src.Airport(q.Origin)
	if got := flight.DateOf(segments[0].Departure.In(origin.Location)); got != q.Date {
		t.Errorf("%s: first departure is on %s local, want %s", name, got, q.Date)
	}

	seen := map[string]bool{segments[0].Origin.Code: true}
	for i, segment := range segments {
		if seen[segment.Destination.Code] {
			t.Errorf("%s: visits %s twice", name, segment.Destination.Code)
		}
		seen[segment.Destination.Code] = true
		if i == 0 {
			continue
		}
		previous, layover := segments[i-1], it.Layovers[i-1]
		if previous.Destination.Code != segment.Origin.Code {
			t.Errorf("%s: lands at %s but continues from %s", name, previous.Destination.Code, segment.Origin.Code)
		}
		want, ok := connect(previous, segment)
		if !ok {
			t.Errorf("%s: illegal connection %s -> %s", name, previous.Number, segment.Number)
		}
		if layover != want {
			t.Errorf("%s: layover %d = %+v, want %+v", name, i-1, layover, want)
		}
	}
}

// key names an itinerary by its flight numbers, e.g. "SP100+SP120".
func key(it flight.Itinerary) string {
	numbers := make([]string, len(it.Segments))
	for i, segment := range it.Segments {
		numbers[i] = segment.Number
	}
	return strings.Join(numbers, "+")
}

func keys(its []flight.Itinerary) []string {
	out := make([]string, len(its))
	for i, it := range its {
		out[i] = key(it)
	}
	return out
}

// assertSameItineraries compares results with the expected keys, ignoring
// order.
func assertSameItineraries(t *testing.T, got []flight.Itinerary, want ...string) {
	t.Helper()
	gotKeys := keys(got)
	slices.Sort(gotKeys)
	want = slices.Sorted(slices.Values(want))
	if !slices.Equal(gotKeys, want) {
		t.Errorf("itineraries = %q, want %q", gotKeys, want)
	}
}

// find returns the itinerary with the given key, failing the test if there
// is none.
func find(t *testing.T, its []flight.Itinerary, want string) flight.Itinerary {
	t.Helper()
	for _, it := range its {
		if key(it) == want {
			return it
		}
	}
	t.Fatalf("no itinerary %s among %q", want, keys(its))
	return flight.Itinerary{}
}

func TestSearchDirect(t *testing.T) {
	src := sourceOf(
		newFlight("SP101", jfk, lax, "2024-03-15 08:30", "2024-03-15 11:45", 350),
		newFlight("SP102", jfk, lax, "2024-03-15 17:00", "2024-03-15 20:15", 300),
		newFlight("SP103", jfk, sfo, "2024-03-15 09:00", "2024-03-15 12:30", 320),
		newFlight("SP104", lax, jfk, "2024-03-15 08:00", "2024-03-15 16:30", 340),
	)
	got := mustSearch(t, src, query(jfk, lax, searchDate))
	assertSameItineraries(t, got, "SP101", "SP102")

	for _, it := range got {
		if it.Layovers == nil {
			t.Errorf("%s: Layovers is nil, want an empty slice", key(it))
		}
	}
	// Local clocks read 08:30 to 11:45; the flight is really 6h15m.
	if direct := find(t, got, "SP101"); direct.Duration() != 375*time.Minute {
		t.Errorf("SP101 duration = %s, want 6h15m", direct.Duration())
	}
}

func TestSearchOneStop(t *testing.T) {
	src := sourceOf(
		newFlight("SP101", jfk, lax, "2024-03-15 08:30", "2024-03-15 11:45", 350),
		newFlight("SP110", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 189),
		newFlight("SP119", ord, lax, "2024-03-15 09:14", "2024-03-15 11:29", 225), // 44 minutes
		newFlight("SP120", ord, lax, "2024-03-15 09:15", "2024-03-15 11:30", 225), // 45 minutes
		newFlight("SP121", ord, lax, "2024-03-15 14:30", "2024-03-15 16:45", 199), // 360 minutes
		newFlight("SP122", ord, lax, "2024-03-15 14:31", "2024-03-15 16:46", 199), // 361 minutes
	)
	got := mustSearch(t, src, query(jfk, lax, searchDate))
	assertSameItineraries(t, got, "SP101", "SP110+SP120", "SP110+SP121")

	it := find(t, got, "SP110+SP120")
	want := flight.Layover{Airport: ord, Duration: 45 * time.Minute, Kind: flight.Domestic}
	if len(it.Layovers) != 1 || it.Layovers[0] != want {
		t.Errorf("layovers = %+v, want [%+v]", it.Layovers, want)
	}
	if it.Duration() != 450*time.Minute || it.Price() != 41400 {
		t.Errorf("duration %s, price %s; want 7h30m, 414.00", it.Duration(), it.Price())
	}
}

func TestSearchTwoStops(t *testing.T) {
	src := sourceOf(
		newFlight("SP200", bos, ord, "2024-03-15 06:00", "2024-03-15 07:45", 150),
		newFlight("SP210", ord, den, "2024-03-15 09:00", "2024-03-15 10:30", 120),
		newFlight("SP220", den, sea, "2024-03-15 11:30", "2024-03-15 12:45", 140),
	)
	got := mustSearch(t, src, query(bos, sea, searchDate))
	assertSameItineraries(t, got, "SP200+SP210+SP220")
	if len(got) != 1 {
		return
	}
	want := []flight.Layover{
		{Airport: ord, Duration: 75 * time.Minute, Kind: flight.Domestic},
		{Airport: den, Duration: 60 * time.Minute, Kind: flight.Domestic},
	}
	if !slices.Equal(got[0].Layovers, want) {
		t.Errorf("layovers = %+v, want %+v", got[0].Layovers, want)
	}
}

func TestSearchNeverReturnsThreeStops(t *testing.T) {
	// BOS-ORD-DEN-SEA-SFO connects legally at every airport, but it is four
	// flights, so SFO is out of reach from BOS.
	src := sourceOf(
		newFlight("SP200", bos, ord, "2024-03-15 06:00", "2024-03-15 07:45", 150),
		newFlight("SP210", ord, den, "2024-03-15 09:00", "2024-03-15 10:30", 120),
		newFlight("SP220", den, sea, "2024-03-15 11:30", "2024-03-15 12:45", 140),
		newFlight("SP230", sea, sfo, "2024-03-15 14:00", "2024-03-15 16:00", 110),
	)
	assertSameItineraries(t, mustSearch(t, src, query(bos, sfo, searchDate)))
	assertSameItineraries(t, mustSearch(t, src, query(ord, sfo, searchDate)), "SP210+SP220+SP230")
}

func TestSearchNeverRevisitsAnAirport(t *testing.T) {
	// Every flight and connection below is legal on its own; what rules some
	// paths out is passing through the origin a second time. Within three
	// flights that is the only revisit possible: the walk stops at the
	// destination, and no flight lands where it took off.
	src := sourceOf(
		newFlight("SP300", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 150),
		newFlight("SP301", ord, jfk, "2024-03-15 09:30", "2024-03-15 12:30", 150),
		newFlight("SP302", jfk, lhr, "2024-03-15 18:00", "2024-03-16 06:00", 500),
		newFlight("SP303", ord, bos, "2024-03-15 10:00", "2024-03-15 13:00", 150),
		newFlight("SP304", bos, ord, "2024-03-15 14:00", "2024-03-15 15:45", 150),
		newFlight("SP305", ord, den, "2024-03-15 17:00", "2024-03-15 18:30", 150),
	)
	// Not JFK-ORD-JFK-LHR.
	assertSameItineraries(t, mustSearch(t, src, query(jfk, lhr, searchDate)), "SP302")
	// Not ORD-BOS-ORD-DEN.
	assertSameItineraries(t, mustSearch(t, src, query(ord, den, searchDate)), "SP305")
	// The same flights are fine when the path does not loop.
	assertSameItineraries(t, mustSearch(t, src, query(ord, lhr, searchDate)), "SP301+SP302")
	assertSameItineraries(t, mustSearch(t, src, query(bos, den, searchDate)), "SP304+SP305")
}

func TestSearchAirportChangeIsNotAConnection(t *testing.T) {
	src := sourceOf(
		newFlight("SP700", ord, lga, "2024-03-15 07:00", "2024-03-15 10:00", 150),
		newFlight("SP701", jfk, lhr, "2024-03-15 13:00", "2024-03-16 01:00", 500),
	)
	assertSameItineraries(t, mustSearch(t, src, query(ord, lhr, searchDate)))
}

func TestSearchOvernightConnection(t *testing.T) {
	// SP400 lands at ORD at 23:00 local; SP401 leaves after midnight, on the
	// next calendar date, and is still a legal 2h30m connection.
	src := sourceOf(
		newFlight("SP400", sfo, ord, "2024-03-15 17:00", "2024-03-15 23:00", 210),
		newFlight("SP401", ord, bos, "2024-03-16 01:30", "2024-03-16 04:45", 130),
	)
	got := mustSearch(t, src, query(sfo, bos, searchDate))
	assertSameItineraries(t, got, "SP400+SP401")
	if len(got) == 1 && got[0].Layovers[0].Duration != 150*time.Minute {
		t.Errorf("layover = %s, want 2h30m", got[0].Layovers[0].Duration)
	}

	// The date belongs to the first departure only.
	assertSameItineraries(t, mustSearch(t, src, query(sfo, bos, searchDate.AddDays(1))))
}

func TestSearchAcrossTheDateLine(t *testing.T) {
	// SP500 lands in Los Angeles at a local time earlier than it left Sydney.
	// Connecting on to SFO is international, because the journey started in
	// Australia, so the 75-minute SP502 is too tight and SP501 at 90 minutes
	// is the earliest legal onward flight.
	src := sourceOf(
		newFlight("SP500", syd, lax, "2024-03-15 09:00", "2024-03-15 06:00", 1100),
		newFlight("SP501", lax, sfo, "2024-03-15 07:30", "2024-03-15 08:55", 120),
		newFlight("SP502", lax, sfo, "2024-03-15 07:15", "2024-03-15 08:40", 120),
	)

	direct := mustSearch(t, src, query(syd, lax, searchDate))
	assertSameItineraries(t, direct, "SP500")
	if len(direct) == 1 && direct[0].Duration() != 15*time.Hour {
		t.Errorf("SYD-LAX duration = %s, want 15h", direct[0].Duration())
	}

	got := mustSearch(t, src, query(syd, sfo, searchDate))
	assertSameItineraries(t, got, "SP500+SP501")
	if len(got) != 1 {
		return
	}
	want := flight.Layover{Airport: lax, Duration: 90 * time.Minute, Kind: flight.International}
	if got[0].Layovers[0] != want {
		t.Errorf("layover = %+v, want %+v", got[0].Layovers[0], want)
	}
	if got[0].Duration() != 17*time.Hour+55*time.Minute {
		t.Errorf("duration = %s, want 17h55m", got[0].Duration())
	}
}

func TestSearchDateIsLocalToTheOrigin(t *testing.T) {
	src := sourceOf(
		newFlight("SP600", lax, sea, "2024-03-14 23:59", "2024-03-15 02:45", 100),
		newFlight("SP601", lax, sea, "2024-03-15 00:00", "2024-03-15 02:46", 100),
		// 23:30 in Los Angeles is already 06:30 on the 16th in UTC.
		newFlight("SP602", lax, sea, "2024-03-15 23:30", "2024-03-16 02:15", 100),
		newFlight("SP603", lax, sea, "2024-03-16 00:15", "2024-03-16 03:00", 100),

		// Sydney is ahead of UTC: 00:30 on the 15th there is still the 14th
		// in UTC, and 23:30 on the 14th belongs to the 14th.
		newFlight("SP610", syd, nrt, "2024-03-14 23:30", "2024-03-15 07:30", 900),
		newFlight("SP611", syd, nrt, "2024-03-15 00:30", "2024-03-15 08:30", 900),
	)
	if utc := local(lax, "2024-03-15 23:30").UTC(); utc.Day() != 16 {
		t.Fatalf("fixture: 23:30 in Los Angeles is %s in UTC, want the 16th", utc)
	}

	cases := []struct {
		name string
		q    flight.Query
		want []string
	}{
		{"LAX on the 14th", query(lax, sea, searchDate.AddDays(-1)), []string{"SP600"}},
		{"LAX on the 15th, from midnight to 23:30", query(lax, sea, searchDate), []string{"SP601", "SP602"}},
		{"LAX on the 16th", query(lax, sea, searchDate.AddDays(1)), []string{"SP603"}},
		{"SYD on the 14th", query(syd, nrt, searchDate.AddDays(-1)), []string{"SP610"}},
		{"SYD on the 15th", query(syd, nrt, searchDate), []string{"SP611"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertSameItineraries(t, mustSearch(t, src, tc.q), tc.want...)
		})
	}
}

func TestSearchRejectsInvalidQueries(t *testing.T) {
	src := sourceOf(newFlight("SP101", jfk, lax, "2024-03-15 08:30", "2024-03-15 11:45", 350))
	cases := []struct {
		name string
		q    flight.Query
	}{
		{"unknown origin", flight.Query{Origin: "XXX", Destination: "LAX", Date: searchDate}},
		{"unknown destination", flight.Query{Origin: "JFK", Destination: "XXX", Date: searchDate}},
		{"empty origin", flight.Query{Origin: "", Destination: "LAX", Date: searchDate}},
		{"same origin and destination", flight.Query{Origin: "JFK", Destination: "JFK", Date: searchDate}},
		{"zero date", flight.Query{Origin: "JFK", Destination: "LAX"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewEngine(src).Search(context.Background(), tc.q)
			if !errors.Is(err, flight.ErrInvalidQuery) {
				t.Errorf("error = %v, want one wrapping flight.ErrInvalidQuery", err)
			}
			if got != nil {
				t.Errorf("itineraries = %q, want nil", keys(got))
			}
		})
	}
}

func TestSearchWithNoMatchesReturnsAnEmptySlice(t *testing.T) {
	src := sourceOf(newFlight("SP101", jfk, lax, "2024-03-15 08:30", "2024-03-15 11:45", 350))
	cases := []struct {
		name string
		q    flight.Query
	}{
		{"date with no flights", query(jfk, lax, flight.Date{Year: 2024, Month: time.March, Day: 20})},
		{"no route", query(lax, jfk, searchDate)},
		{"airport with no departures", query(sea, jfk, searchDate)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mustSearch(t, src, tc.q); len(got) != 0 {
				t.Errorf("itineraries = %q, want none", keys(got))
			}
		})
	}
}
