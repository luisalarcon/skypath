package web

import (
	"reflect"
	"testing"

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/view"
)

// loadSearch decodes a canonical search payload from api/examples.
func loadSearch(t *testing.T, name string) *client.SearchResponse {
	t.Helper()
	var resp client.SearchResponse
	loadExample(t, name, &resp)
	return &resp
}

func TestResultsViewFromSuccessExample(t *testing.T) {
	got := resultsView(loadSearch(t, "search-success.json"))

	if got.Origin != (view.Airport{Code: "JFK", Name: "John F. Kennedy International", City: "New York", Country: "US"}) {
		t.Errorf("origin = %+v", got.Origin)
	}
	if got.Destination.Code != "LAX" || got.Destination.City != "Los Angeles" {
		t.Errorf("destination = %+v", got.Destination)
	}
	if got.Date != "Friday, March 15, 2024" || got.DateISO != "2024-03-15" {
		t.Errorf("date = %q / %q", got.Date, got.DateISO)
	}
	if got.Count != 2 || got.Direct != 1 || got.OneStop != 1 || got.TwoStop != 0 {
		t.Errorf("counts = %d total, %d direct, %d one-stop, %d two-stop", got.Count, got.Direct, got.OneStop, got.TwoStop)
	}
	if len(got.Itineraries) != 2 {
		t.Fatalf("itineraries = %d, want 2", len(got.Itineraries))
	}

	direct := got.Itineraries[0]
	if direct.Stops != 0 || direct.StopsLabel != "Direct" || len(direct.Via) != 0 {
		t.Errorf("direct stops = %d %q via %v", direct.Stops, direct.StopsLabel, direct.Via)
	}
	if !reflect.DeepEqual(direct.Tags, []string{"Fastest", "Cheapest"}) {
		t.Errorf("direct tags = %v, want Fastest and Cheapest", direct.Tags)
	}
	if direct.Duration != "6h 15m" || direct.DurationMinutes != 375 || direct.Price != "$299.00" || direct.PriceCents != 29900 {
		t.Errorf("direct totals = %q %d %q %d", direct.Duration, direct.DurationMinutes, direct.Price, direct.PriceCents)
	}
	wantDeparture := view.TimePoint{
		Time: "8:30 AM", Date: "Fri, Mar 15", ISO: "2024-03-15T08:30:00-04:00", UTCOffset: "UTC-4",
		Airport: view.Airport{Code: "JFK", Name: "John F. Kennedy International", City: "New York", Country: "US"},
	}
	if direct.Departure != wantDeparture {
		t.Errorf("direct departure = %+v, want %+v", direct.Departure, wantDeparture)
	}
	wantArrival := view.TimePoint{
		Time: "11:45 AM", Date: "Fri, Mar 15", ISO: "2024-03-15T11:45:00-07:00", UTCOffset: "UTC-7",
		Airport: view.Airport{Code: "LAX", Name: "Los Angeles International", City: "Los Angeles", Country: "US"},
	}
	if direct.Arrival != wantArrival {
		t.Errorf("direct arrival = %+v, want %+v", direct.Arrival, wantArrival)
	}
	if direct.DayOffset != "" {
		t.Errorf("direct day offset = %q, want none", direct.DayOffset)
	}
	if len(direct.Segments) != 1 || direct.Segments[0].Layover != nil {
		t.Errorf("direct segments = %+v", direct.Segments)
	}

	oneStop := got.Itineraries[1]
	if oneStop.StopsLabel != "1 stop" || !reflect.DeepEqual(oneStop.Via, []string{"ORD"}) {
		t.Errorf("one-stop = %q via %v", oneStop.StopsLabel, oneStop.Via)
	}
	if len(oneStop.Tags) != 0 {
		t.Errorf("one-stop tags = %v, want none", oneStop.Tags)
	}
	if oneStop.Duration != "7h 30m" || oneStop.Price != "$414.00" || oneStop.PriceCents != 41400 {
		t.Errorf("one-stop totals = %q %q %d", oneStop.Duration, oneStop.Price, oneStop.PriceCents)
	}
	if len(oneStop.Segments) != 2 {
		t.Fatalf("one-stop segments = %d, want 2", len(oneStop.Segments))
	}

	first, second := oneStop.Segments[0], oneStop.Segments[1]
	if first.FlightNumber != "SP110" || first.Airline != "SkyPath Airways" || first.Aircraft != "A319" ||
		first.Duration != "2h 30m" || first.Price != "$189.00" || first.DayOffset != "" {
		t.Errorf("first segment = %+v", first)
	}
	if first.Departure.Time != "7:00 AM" || first.Departure.Airport.Code != "JFK" ||
		first.Arrival.Time != "8:30 AM" || first.Arrival.UTCOffset != "UTC-5" || first.Arrival.Airport.Code != "ORD" {
		t.Errorf("first segment times = %+v → %+v", first.Departure, first.Arrival)
	}
	wantLayover := &view.Layover{
		Airport:  view.Airport{Code: "ORD", Name: "O'Hare International", City: "Chicago", Country: "US"},
		Duration: "45m", Kind: "domestic", Label: "Domestic connection",
	}
	if !reflect.DeepEqual(first.Layover, wantLayover) {
		t.Errorf("first layover = %+v, want %+v", first.Layover, wantLayover)
	}
	if second.Layover != nil {
		t.Errorf("last segment has a layover: %+v", second.Layover)
	}
	if second.FlightNumber != "SP120" || second.Duration != "4h 15m" || second.Departure.Time != "9:15 AM" || second.Arrival.Time != "11:30 AM" {
		t.Errorf("second segment = %+v", second)
	}

	// The itinerary's end points are those of its first and last flights.
	if oneStop.Departure.Airport.Code != "JFK" || oneStop.Arrival.Airport.Code != "LAX" {
		t.Errorf("one-stop end airports = %s → %s", oneStop.Departure.Airport.Code, oneStop.Arrival.Airport.Code)
	}
}

func TestResultsViewFromTwoStopExample(t *testing.T) {
	got := resultsView(loadSearch(t, "search-two-stops.json"))

	if got.Count != 1 || got.TwoStop != 1 || got.Direct != 0 || got.OneStop != 0 {
		t.Errorf("counts = %d total, %d direct, %d one-stop, %d two-stop", got.Count, got.Direct, got.OneStop, got.TwoStop)
	}
	it := got.Itineraries[0]

	if it.StopsLabel != "2 stops" || !reflect.DeepEqual(it.Via, []string{"DEN", "LAX"}) {
		t.Errorf("stops = %q via %v", it.StopsLabel, it.Via)
	}
	if !reflect.DeepEqual(it.Tags, []string{"Fastest", "Cheapest"}) {
		t.Errorf("a lone itinerary is both fastest and cheapest, got %v", it.Tags)
	}
	if it.Duration != "21h" || it.Price != "$1,137.00" || it.PriceCents != 113700 {
		t.Errorf("totals = %q %q %d", it.Duration, it.Price, it.PriceCents)
	}
	if it.Departure.Time != "2:00 PM" || it.Departure.UTCOffset != "UTC-7" || it.Departure.Airport.Code != "SFO" {
		t.Errorf("departure = %+v", it.Departure)
	}
	if it.Arrival.Time != "3:00 AM" || it.Arrival.Date != "Sun, Mar 17" || it.Arrival.UTCOffset != "UTC+9" ||
		it.Arrival.ISO != "2024-03-17T03:00:00+09:00" || it.Arrival.Airport.Code != "NRT" {
		t.Errorf("arrival = %+v", it.Arrival)
	}
	if it.DayOffset != "+2 days" {
		t.Errorf("itinerary day offset = %q, want +2 days", it.DayOffset)
	}

	if len(it.Segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(it.Segments))
	}
	wantOffsets := []string{"", "", "+2 days"}
	wantKinds := []string{"domestic", "international", ""}
	for i, seg := range it.Segments {
		if seg.DayOffset != wantOffsets[i] {
			t.Errorf("segment %d day offset = %q, want %q", i, seg.DayOffset, wantOffsets[i])
		}
		switch {
		case wantKinds[i] == "" && seg.Layover != nil:
			t.Errorf("segment %d has a layover: %+v", i, seg.Layover)
		case wantKinds[i] != "" && (seg.Layover == nil || seg.Layover.Kind != wantKinds[i]):
			t.Errorf("segment %d layover = %+v, want kind %s", i, seg.Layover, wantKinds[i])
		}
	}
	if l := it.Segments[0].Layover; l.Duration != "1h 30m" || l.Airport.Code != "DEN" || l.Label != "Domestic connection" {
		t.Errorf("first layover = %+v", l)
	}
	if l := it.Segments[1].Layover; l.Duration != "2h 45m" || l.Label != "International connection" {
		t.Errorf("second layover = %+v", l)
	}
	if last := it.Segments[2]; last.Duration != "12h" || last.Price != "$799.00" || last.Arrival.Date != "Sun, Mar 17" {
		t.Errorf("last segment = %+v", last)
	}
}

func TestResultsViewFromEmptyExample(t *testing.T) {
	got := resultsView(loadSearch(t, "search-empty.json"))

	if got.Count != 0 || got.Direct != 0 || got.OneStop != 0 || got.TwoStop != 0 {
		t.Errorf("counts = %+v, want all zero", got)
	}
	if got.Itineraries == nil || len(got.Itineraries) != 0 {
		t.Errorf("itineraries = %#v, want an empty slice", got.Itineraries)
	}
	if got.Date != "Wednesday, March 20, 2024" || got.DateISO != "2024-03-20" {
		t.Errorf("date = %q / %q", got.Date, got.DateISO)
	}
}

func TestResultsViewKeepsUnparseableDate(t *testing.T) {
	resp := loadSearch(t, "search-empty.json")
	resp.Date = "someday"

	got := resultsView(resp)
	if got.Date != "someday" || got.DateISO != "someday" {
		t.Errorf("date = %q / %q, want the value as received", got.Date, got.DateISO)
	}
}

func TestAddTags(t *testing.T) {
	tests := []struct {
		name   string
		prices []int64
		want   [][]string
	}{
		{"no itineraries", nil, nil},
		{"single itinerary is both", []int64{500}, [][]string{{"Fastest", "Cheapest"}}},
		{"fastest is also cheapest", []int64{100, 200, 300}, [][]string{{"Fastest", "Cheapest"}, nil, nil}},
		{"cheapest is later", []int64{300, 100, 200}, [][]string{{"Fastest"}, {"Cheapest"}, nil}},
		{"cheapest is last", []int64{300, 200, 100}, [][]string{{"Fastest"}, nil, {"Cheapest"}}},
		{"tie goes to the first", []int64{300, 100, 100}, [][]string{{"Fastest"}, {"Cheapest"}, nil}},
		{"all tied", []int64{100, 100, 100}, [][]string{{"Fastest", "Cheapest"}, nil, nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			its := make([]view.Itinerary, len(tt.prices))
			for i, p := range tt.prices {
				its[i].PriceCents = p
			}
			addTags(its)
			for i := range its {
				if !reflect.DeepEqual(its[i].Tags, tt.want[i]) {
					t.Errorf("itinerary %d tags = %v, want %v", i, its[i].Tags, tt.want[i])
				}
			}
		})
	}
}

func TestItineraryViewWithoutSegmentsUsesSearchAirports(t *testing.T) {
	origin := client.Airport{Code: "JFK"}
	destination := client.Airport{Code: "LAX"}

	got := itineraryView(client.Itinerary{}, origin, destination)
	if got.Departure.Airport.Code != "JFK" || got.Arrival.Airport.Code != "LAX" {
		t.Errorf("end airports = %s → %s", got.Departure.Airport.Code, got.Arrival.Airport.Code)
	}
	if got.Segments == nil || len(got.Segments) != 0 || got.Via == nil {
		t.Errorf("segments and via should be empty, not nil: %#v %#v", got.Segments, got.Via)
	}
}
