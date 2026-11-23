package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/config"
)

// These tests run the whole backend, wired exactly as in production, against
// the shipped schedule and through its HTTP API. The expected numbers are the
// reference results in docs/testing.md, which were produced by a separate
// implementation of the rules.

const schedulePath = "../../../flights.json"

type airportJSON struct {
	Code    string `json:"code"`
	Country string `json:"country"`
}

type segmentJSON struct {
	FlightNumber    string      `json:"flightNumber"`
	Origin          airportJSON `json:"origin"`
	Destination     airportJSON `json:"destination"`
	DepartureTime   time.Time   `json:"departureTime"`
	ArrivalTime     time.Time   `json:"arrivalTime"`
	DurationMinutes int         `json:"durationMinutes"`
	Price           float64     `json:"price"`
}

type layoverJSON struct {
	Airport         airportJSON `json:"airport"`
	DurationMinutes int         `json:"durationMinutes"`
	Type            string      `json:"type"`
}

type itineraryJSON struct {
	Stops                int           `json:"stops"`
	DepartureTime        time.Time     `json:"departureTime"`
	ArrivalTime          time.Time     `json:"arrivalTime"`
	TotalDurationMinutes int           `json:"totalDurationMinutes"`
	TotalPrice           float64       `json:"totalPrice"`
	Segments             []segmentJSON `json:"segments"`
	Layovers             []layoverJSON `json:"layovers"`
}

type searchJSON struct {
	Count       int             `json:"count"`
	Itineraries []itineraryJSON `json:"itineraries"`
}

type problemJSON struct {
	Code   string `json:"code"`
	Errors []struct {
		Field   string `json:"field"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.Config{FlightsPath: schedulePath, RequestTimeout: 10 * time.Second}
	handler, err := newHandler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("wire the service: %v", err)
	}
	return handler
}

func get(t *testing.T, handler http.Handler, target string, into any) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	if into != nil {
		if err := json.Unmarshal(recorder.Body.Bytes(), into); err != nil {
			t.Fatalf("GET %s: decode body: %v\n%s", target, err, recorder.Body)
		}
	}
	return recorder.Code
}

func searchURL(origin, destination, date string) string {
	query := url.Values{"origin": {origin}, "destination": {destination}, "date": {date}}
	return "/api/v1/search?" + query.Encode()
}

func runSearch(t *testing.T, handler http.Handler, origin, destination, date string) searchJSON {
	t.Helper()
	var result searchJSON
	if status := get(t, handler, searchURL(origin, destination, date), &result); status != http.StatusOK {
		t.Fatalf("search %s to %s on %s: status %d, want 200", origin, destination, date, status)
	}
	return result
}

func flightNumbers(it itineraryJSON) string {
	numbers := make([]string, len(it.Segments))
	for i, segment := range it.Segments {
		numbers[i] = segment.FlightNumber
	}
	return strings.Join(numbers, " ")
}

func cents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func minutesBetween(from, to time.Time) int {
	return int(to.Sub(from) / time.Minute)
}

func TestReferenceSearches(t *testing.T) {
	handler := newTestHandler(t)

	cases := []struct {
		origin, destination string
		total               int
		byStops             [3]int // direct, one stop, two stops
		fastestMinutes      int
	}{
		{"JFK", "LAX", 27, [3]int{3, 14, 10}, 375},
		{"SFO", "NRT", 6, [3]int{2, 1, 3}, 720},  // international: 90-minute minimum applies
		{"BOS", "SEA", 13, [3]int{0, 4, 9}, 630}, // no direct flight exists
		{"SYD", "LAX", 2, [3]int{2, 0, 0}, 900},  // crosses the date line
	}
	for _, tc := range cases {
		t.Run(tc.origin+"-"+tc.destination, func(t *testing.T) {
			result := runSearch(t, handler, tc.origin, tc.destination, "2024-03-15")

			if result.Count != tc.total || len(result.Itineraries) != tc.total {
				t.Fatalf("count = %d with %d itineraries, want %d", result.Count, len(result.Itineraries), tc.total)
			}
			var byStops [3]int
			for _, it := range result.Itineraries {
				byStops[it.Stops]++
			}
			if byStops != tc.byStops {
				t.Errorf("direct/one-stop/two-stop = %v, want %v", byStops, tc.byStops)
			}
			if got := result.Itineraries[0].TotalDurationMinutes; got != tc.fastestMinutes {
				t.Errorf("fastest itinerary = %d minutes, want %d", got, tc.fastestMinutes)
			}
		})
	}
}

func TestReferenceItineraries(t *testing.T) {
	handler := newTestHandler(t)

	type layover struct {
		airport string
		minutes int
		kind    string
	}
	cases := []struct {
		origin, destination string
		flights             string
		minutes             int
		priceCents          int64
		layovers            []layover
	}{
		// A domestic connection sitting exactly on the 45-minute minimum.
		{"JFK", "LAX", "SP110 SP120", 450, 41400, []layover{{"ORD", 45, "domestic"}}},
		// An international connection sitting exactly on the 90-minute minimum.
		{"SFO", "NRT", "SP998 SP500", 900, 94400, []layover{{"LAX", 90, "international"}}},
		// Two stops, one of each kind, arriving two calendar days later.
		{"SFO", "NRT", "SP981 SP212 SP501", 1260, 113700, []layover{{"DEN", 90, "domestic"}, {"LAX", 165, "international"}}},
		{"BOS", "SEA", "SP260 SP221", 630, 42800, []layover{{"DEN", 180, "domestic"}}},
		// Lands at an earlier local time than it departs, on the same date.
		{"SYD", "LAX", "SP540", 900, 109900, nil},
	}
	for _, tc := range cases {
		t.Run(tc.flights, func(t *testing.T) {
			result := runSearch(t, handler, tc.origin, tc.destination, "2024-03-15")
			index := slices.IndexFunc(result.Itineraries, func(it itineraryJSON) bool {
				return flightNumbers(it) == tc.flights
			})
			if index < 0 {
				t.Fatalf("itinerary %q not found", tc.flights)
			}
			it := result.Itineraries[index]

			if it.TotalDurationMinutes != tc.minutes {
				t.Errorf("duration = %d minutes, want %d", it.TotalDurationMinutes, tc.minutes)
			}
			if got := cents(it.TotalPrice); got != tc.priceCents {
				t.Errorf("price = %d cents, want %d", got, tc.priceCents)
			}
			if len(it.Layovers) != len(tc.layovers) {
				t.Fatalf("layovers = %d, want %d", len(it.Layovers), len(tc.layovers))
			}
			for i, want := range tc.layovers {
				got := it.Layovers[i]
				if got.Airport.Code != want.airport || got.DurationMinutes != want.minutes || got.Type != want.kind {
					t.Errorf("layover %d = %s %d min %s, want %s %d min %s",
						i, got.Airport.Code, got.DurationMinutes, got.Type, want.airport, want.minutes, want.kind)
				}
			}
		})
	}
}

func TestDateLineArrivalReadsEarlierThanDeparture(t *testing.T) {
	handler := newTestHandler(t)
	result := runSearch(t, handler, "SYD", "LAX", "2024-03-15")

	for _, it := range result.Itineraries {
		segment := it.Segments[0]
		departure := segment.DepartureTime.Format("2006-01-02T15:04")
		arrival := segment.ArrivalTime.Format("2006-01-02T15:04")
		if arrival >= departure {
			t.Errorf("%s: local arrival %s should read earlier than local departure %s", segment.FlightNumber, arrival, departure)
		}
		if segment.DurationMinutes != 900 {
			t.Errorf("%s: duration = %d minutes, want 900", segment.FlightNumber, segment.DurationMinutes)
		}
	}
}

func TestInvalidSearches(t *testing.T) {
	handler := newTestHandler(t)

	cases := []struct {
		name                string
		origin, destination string
		field, code         string
		message             string
	}{
		{"same airport", "JFK", "JFK", "destination", "same_as_origin", "Destination must be different from origin."},
		{"unknown airport", "XXX", "LAX", "origin", "unknown_airport", "Unknown airport code XXX."},
		// The schedule contains a record with this misspelt origin. It must be
		// neither loaded nor searchable.
		{"misspelt code from the schedule", "JKF", "LAX", "origin", "unknown_airport", "Unknown airport code JKF."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var problem problemJSON
			status := get(t, handler, searchURL(tc.origin, tc.destination, "2024-03-15"), &problem)

			if status != http.StatusBadRequest || problem.Code != "validation_failed" {
				t.Fatalf("status %d code %q, want 400 validation_failed", status, problem.Code)
			}
			if len(problem.Errors) != 1 {
				t.Fatalf("errors = %+v, want exactly one", problem.Errors)
			}
			got := problem.Errors[0]
			if got.Field != tc.field || got.Code != tc.code || got.Message != tc.message {
				t.Errorf("error = %+v, want %s %s %q", got, tc.field, tc.code, tc.message)
			}
		})
	}
}

func TestSearchOutsideTheScheduleIsEmptyNotAnError(t *testing.T) {
	handler := newTestHandler(t)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, searchURL("JFK", "LAX", "2024-03-20"), nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"itineraries":[]`) {
		t.Errorf("body should carry an empty list, not null: %s", recorder.Body)
	}
}

func TestSkippedRecordIsReportedAndNeverServed(t *testing.T) {
	handler := newTestHandler(t)

	var ready struct {
		Status  string `json:"status"`
		Dataset struct {
			Airports int `json:"airports"`
			Flights  int `json:"flights"`
			Skipped  []struct {
				Record string `json:"record"`
				Reason string `json:"reason"`
			} `json:"skipped"`
		} `json:"dataset"`
	}
	if status := get(t, handler, "/readyz", &ready); status != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200", status)
	}
	if ready.Dataset.Airports != 25 || ready.Dataset.Flights != 302 {
		t.Errorf("loaded %d airports and %d flights, want 25 and 302", ready.Dataset.Airports, ready.Dataset.Flights)
	}
	if len(ready.Dataset.Skipped) != 1 || ready.Dataset.Skipped[0].Record != "flight SP995" ||
		!strings.Contains(ready.Dataset.Skipped[0].Reason, "JKF") {
		t.Errorf("skipped = %+v, want only flight SP995 naming JKF", ready.Dataset.Skipped)
	}

	for _, it := range runSearch(t, handler, "JFK", "LAX", "2024-03-15").Itineraries {
		if strings.Contains(flightNumbers(it), "SP995") {
			t.Errorf("skipped flight SP995 appears in results: %s", flightNumbers(it))
		}
	}

	var info struct {
		First string `json:"firstDepartureDate"`
		Last  string `json:"lastDepartureDate"`
	}
	get(t, handler, "/api/v1/dataset", &info)
	if info.First != "2024-03-15" || info.Last != "2024-03-16" {
		t.Errorf("departure dates = %s to %s, want 2024-03-15 to 2024-03-16", info.First, info.Last)
	}
}

// TestEveryItineraryObeysTheRules searches every ordered pair of airports on
// every date the schedule covers and checks each returned itinerary against
// the rules in docs/search.md, using only what the API returns. It cannot
// tell whether an itinerary is missing (the reference counts do that), but it
// proves nothing illegal is ever returned.
func TestEveryItineraryObeysTheRules(t *testing.T) {
	handler := newTestHandler(t)

	var directory struct {
		Airports []airportJSON `json:"airports"`
	}
	get(t, handler, "/api/v1/airports", &directory)
	if len(directory.Airports) != 25 {
		t.Fatalf("airports = %d, want 25", len(directory.Airports))
	}

	const (
		minDomestic      = 45
		minInternational = 90
		maxLayover       = 360
	)
	total := 0
	shortestDomestic, shortestInternational, longest := math.MaxInt, math.MaxInt, 0

	for _, date := range []string{"2024-03-15", "2024-03-16"} {
		for _, origin := range directory.Airports {
			for _, destination := range directory.Airports {
				if origin.Code == destination.Code {
					continue
				}
				result := runSearch(t, handler, origin.Code, destination.Code, date)
				total += len(result.Itineraries)

				for i, it := range result.Itineraries {
					name := origin.Code + "-" + destination.Code + " " + date + " [" + flightNumbers(it) + "]"

					if n := len(it.Segments); n < 1 || n > 3 || it.Stops != n-1 || len(it.Layovers) != n-1 {
						t.Fatalf("%s: %d segments, %d stops, %d layovers", name, n, it.Stops, len(it.Layovers))
					}
					first, last := it.Segments[0], it.Segments[len(it.Segments)-1]

					if first.Origin.Code != origin.Code || last.Destination.Code != destination.Code {
						t.Errorf("%s: runs %s to %s", name, first.Origin.Code, last.Destination.Code)
					}
					if got := first.DepartureTime.Format("2006-01-02"); got != date {
						t.Errorf("%s: first departure is on %s local time", name, got)
					}

					visited := map[string]bool{first.Origin.Code: true}
					var priceCents int64
					for _, segment := range it.Segments {
						if visited[segment.Destination.Code] {
							t.Errorf("%s: visits %s twice", name, segment.Destination.Code)
						}
						visited[segment.Destination.Code] = true
						priceCents += cents(segment.Price)

						if minutes := minutesBetween(segment.DepartureTime, segment.ArrivalTime); minutes <= 0 || minutes != segment.DurationMinutes {
							t.Errorf("%s: %s reports %d minutes, times give %d", name, segment.FlightNumber, segment.DurationMinutes, minutes)
						}
					}

					for j, layover := range it.Layovers {
						before, after := it.Segments[j], it.Segments[j+1]
						if before.Destination.Code != after.Origin.Code || layover.Airport.Code != after.Origin.Code {
							t.Errorf("%s: connection %d changes airport: lands %s, leaves %s", name, j, before.Destination.Code, after.Origin.Code)
						}
						minutes := minutesBetween(before.ArrivalTime, after.DepartureTime)
						if minutes != layover.DurationMinutes {
							t.Errorf("%s: layover %d reports %d minutes, times give %d", name, j, layover.DurationMinutes, minutes)
						}

						domestic := before.Origin.Country == before.Destination.Country &&
							before.Destination.Country == after.Destination.Country
						wantKind, minimum := "international", minInternational
						if domestic {
							wantKind, minimum = "domestic", minDomestic
						}
						if layover.Type != wantKind {
							t.Errorf("%s: layover %d is %s, want %s", name, j, layover.Type, wantKind)
						}
						if minutes < minimum || minutes > maxLayover {
							t.Errorf("%s: %s layover of %d minutes is outside %d to %d", name, wantKind, minutes, minimum, maxLayover)
						}

						if domestic {
							shortestDomestic = min(shortestDomestic, minutes)
						} else {
							shortestInternational = min(shortestInternational, minutes)
						}
						longest = max(longest, minutes)
					}

					if !it.DepartureTime.Equal(first.DepartureTime) || !it.ArrivalTime.Equal(last.ArrivalTime) {
						t.Errorf("%s: itinerary times do not match its first and last segments", name)
					}
					if minutes := minutesBetween(first.DepartureTime, last.ArrivalTime); minutes != it.TotalDurationMinutes {
						t.Errorf("%s: reports %d minutes in total, times give %d", name, it.TotalDurationMinutes, minutes)
					}
					if cents(it.TotalPrice) != priceCents {
						t.Errorf("%s: total price %d cents, segments sum to %d", name, cents(it.TotalPrice), priceCents)
					}

					if i > 0 {
						previous := result.Itineraries[i-1]
						outOfOrder := previous.TotalDurationMinutes > it.TotalDurationMinutes ||
							(previous.TotalDurationMinutes == it.TotalDurationMinutes && cents(previous.TotalPrice) > cents(it.TotalPrice))
						if outOfOrder {
							t.Errorf("%s: sorted after a slower or pricier itinerary", name)
						}
					}
				}
			}
		}
	}

	if total == 0 {
		t.Fatal("no itineraries were returned for any route")
	}
	// The schedule has connections sitting exactly on each limit. Seeing them
	// in the results proves the limits are inclusive on real data.
	if shortestDomestic != minDomestic || shortestInternational != minInternational || longest != maxLayover {
		t.Errorf("layover extremes: domestic %d, international %d, longest %d; want %d, %d and %d",
			shortestDomestic, shortestInternational, longest, minDomestic, minInternational, maxLayover)
	}
	t.Logf("checked %d itineraries across %d routes and 2 dates", total, len(directory.Airports)*(len(directory.Airports)-1))
}
