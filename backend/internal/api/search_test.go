package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// searchFixture wires a handler to a recording searcher.
type searchFixture struct {
	handler http.Handler
	calls   int
	last    flight.Query
	logs    func() []map[string]any
}

// newSearchFixture returns a handler whose searcher runs fn and records how
// it was called. The directory knows JFK, LAX and ORD.
func newSearchFixture(t *testing.T, timeout time.Duration, fn searcherFunc) *searchFixture {
	t.Helper()
	a := newAirports(t)
	logger, buf := testLogger()
	f := &searchFixture{logs: func() []map[string]any { return logRecords(t, buf) }}
	f.handler = NewHandler(Dependencies{
		Search: searcherFunc(func(ctx context.Context, q flight.Query) ([]flight.Itinerary, error) {
			f.calls++
			f.last = q
			return fn(ctx, q)
		}),
		Airports:       stubDirectory{a.JFK, a.LAX, a.ORD},
		Dataset:        sampleDataset(),
		Logger:         logger,
		RequestTimeout: timeout,
	})
	return f
}

func noItineraries(context.Context, flight.Query) ([]flight.Itinerary, error) {
	return nil, nil
}

func decodeProblem(t *testing.T, body []byte) problem {
	t.Helper()
	var p problem
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("body is not a problem document: %q: %v", body, err)
	}
	return p
}

func TestSearchValidation(t *testing.T) {
	const okDate = "2024-03-15"
	invalidDate := fieldError{"date", "invalid_format", "Date must be a valid date in YYYY-MM-DD format."}
	dateRequired := fieldError{"date", "required", "Date is required."}
	originRequired := fieldError{"origin", "required", "Origin is required."}
	destRequired := fieldError{"destination", "required", "Destination is required."}
	originFormat := fieldError{"origin", "invalid_format", "Origin must be a three-letter airport code."}
	destFormat := fieldError{"destination", "invalid_format", "Destination must be a three-letter airport code."}
	sameAsOrigin := fieldError{"destination", "same_as_origin", "Destination must be different from origin."}
	unknown := func(field, code string) fieldError {
		return fieldError{field, "unknown_airport", "Unknown airport code " + code + "."}
	}

	tests := []struct {
		name  string
		query string
		want  []fieldError
	}{
		// Origin and destination: required.
		{"origin missing", "destination=LAX&date=" + okDate, []fieldError{originRequired}},
		{"origin blank", "origin=&destination=LAX&date=" + okDate, []fieldError{originRequired}},
		{"origin only spaces", "origin=%20%20%20&destination=LAX&date=" + okDate, []fieldError{originRequired}},
		{"destination missing", "origin=JFK&date=" + okDate, []fieldError{destRequired}},
		{"destination blank", "origin=JFK&destination=&date=" + okDate, []fieldError{destRequired}},
		{"destination only spaces", "origin=JFK&destination=%20&date=" + okDate, []fieldError{destRequired}},

		// Origin and destination: format.
		{"origin too short", "origin=JF&destination=LAX&date=" + okDate, []fieldError{originFormat}},
		{"origin too long", "origin=JFKK&destination=LAX&date=" + okDate, []fieldError{originFormat}},
		{"origin with digit", "origin=J1K&destination=LAX&date=" + okDate, []fieldError{originFormat}},
		{"origin all digits", "origin=123&destination=LAX&date=" + okDate, []fieldError{originFormat}},
		{"origin with inner space", "origin=J%20K&destination=LAX&date=" + okDate, []fieldError{originFormat}},
		{"origin non-ASCII letters", "origin=%C3%84BC&destination=LAX&date=" + okDate, []fieldError{originFormat}},
		{"destination too short", "origin=JFK&destination=LA&date=" + okDate, []fieldError{destFormat}},
		{"destination too long", "origin=JFK&destination=LAXX&date=" + okDate, []fieldError{destFormat}},
		{"destination with symbol", "origin=JFK&destination=L-X&date=" + okDate, []fieldError{destFormat}},

		// Origin and destination: unknown.
		{"origin unknown", "origin=XXX&destination=LAX&date=" + okDate, []fieldError{unknown("origin", "XXX")}},
		{"origin unknown lower case reports upper case", "origin=xxx&destination=LAX&date=" + okDate, []fieldError{unknown("origin", "XXX")}},
		{"destination unknown", "origin=JFK&destination=ZZZ&date=" + okDate, []fieldError{unknown("destination", "ZZZ")}},
		{"known airport outside the directory", "origin=JFK&destination=NRT&date=" + okDate, []fieldError{unknown("destination", "NRT")}},

		// Date.
		{"date missing", "origin=JFK&destination=LAX", []fieldError{dateRequired}},
		{"date blank", "origin=JFK&destination=LAX&date=", []fieldError{dateRequired}},
		{"date only spaces", "origin=JFK&destination=LAX&date=%20", []fieldError{dateRequired}},
		{"date impossible", "origin=JFK&destination=LAX&date=2024-02-30", []fieldError{invalidDate}},
		{"date month 13", "origin=JFK&destination=LAX&date=2024-13-01", []fieldError{invalidDate}},
		{"date not a leap day", "origin=JFK&destination=LAX&date=2023-02-29", []fieldError{invalidDate}},
		{"date without zero padding", "origin=JFK&destination=LAX&date=2024-3-15", []fieldError{invalidDate}},
		{"date day first", "origin=JFK&destination=LAX&date=15-03-2024", []fieldError{invalidDate}},
		{"date with slashes", "origin=JFK&destination=LAX&date=2024/03/15", []fieldError{invalidDate}},
		{"date with time", "origin=JFK&destination=LAX&date=2024-03-15T00:00:00", []fieldError{invalidDate}},
		{"date is words", "origin=JFK&destination=LAX&date=tomorrow", []fieldError{invalidDate}},
		{"date with padding is not trimmed", "origin=JFK&destination=LAX&date=%202024-03-15", []fieldError{invalidDate}},

		// Same airport.
		{"same origin and destination", "origin=JFK&destination=JFK&date=" + okDate, []fieldError{sameAsOrigin}},
		{"same after normalising", "origin=%20jfk%20&destination=JFK&date=" + okDate, []fieldError{sameAsOrigin}},
		{"same airport and bad date", "origin=JFK&destination=JFK&date=nope", []fieldError{sameAsOrigin, invalidDate}},
		{"same but unknown reports each code, not the repeat", "origin=XXX&destination=XXX&date=" + okDate,
			[]fieldError{unknown("origin", "XXX"), unknown("destination", "XXX")}},
		{"same but malformed reports each code, not the repeat", "origin=J1K&destination=J1K&date=" + okDate,
			[]fieldError{originFormat, destFormat}},

		// Several problems at once, in the documented order.
		{"nothing supplied", "", []fieldError{originRequired, destRequired, dateRequired}},
		{"every field wrong", "origin=XXX&destination=J&date=nope", []fieldError{unknown("origin", "XXX"), destFormat, invalidDate}},
		{"origin and date wrong", "origin=XXX&destination=LAX&date=15-03-2024", []fieldError{unknown("origin", "XXX"), invalidDate}},
		{"destination and date missing", "origin=JFK", []fieldError{destRequired, dateRequired}},

		// Repeated parameters: the first value is used.
		{"repeated origin uses the first", "origin=XXX&origin=JFK&destination=LAX&date=" + okDate, []fieldError{unknown("origin", "XXX")}},
		{"repeated origin with blank first", "origin=&origin=JFK&destination=LAX&date=" + okDate, []fieldError{originRequired}},
		{"repeated date uses the first", "origin=JFK&destination=LAX&date=nope&date=" + okDate, []fieldError{invalidDate}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newSearchFixture(t, 0, noItineraries)
			rec := get(f.handler, "/api/v1/search?"+tc.query, "X-Request-ID", "req-400")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			p := decodeProblem(t, rec.Body.Bytes())
			if p.Code != "validation_failed" || p.Title != "Bad Request" || p.Detail != "The search request is invalid." || p.RequestID != "req-400" {
				t.Errorf("problem = %+v", p)
			}
			if !reflect.DeepEqual(p.Errors, tc.want) {
				t.Errorf("errors =\n  %v\nwant\n  %v", p.Errors, tc.want)
			}
			if f.calls != 0 {
				t.Error("an invalid request must not reach the searcher")
			}
		})
	}
}

func TestSearchNormalisesInput(t *testing.T) {
	want := flight.Query{Origin: "JFK", Destination: "LAX", Date: flight.Date{Year: 2024, Month: time.March, Day: 15}}

	tests := []struct {
		name  string
		query string
	}{
		{"canonical", "origin=JFK&destination=LAX&date=2024-03-15"},
		{"lower case", "origin=jfk&destination=lax&date=2024-03-15"},
		{"mixed case", "origin=JfK&destination=lAx&date=2024-03-15"},
		{"padded", "origin=%20JFK%20&destination=%09LAX&date=2024-03-15"},
		{"padded and lower case", "origin=%20jfk%20&destination=%20lax%20&date=2024-03-15"},
		{"repeated parameters use the first", "origin=JFK&origin=XXX&destination=LAX&destination=&date=2024-03-15&date=bad"},
		{"unknown parameters are ignored", "origin=JFK&destination=LAX&date=2024-03-15&sort=price&limit=1"},
		{"parameter order does not matter", "date=2024-03-15&destination=LAX&origin=JFK"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newSearchFixture(t, 0, noItineraries)
			rec := get(f.handler, "/api/v1/search?"+tc.query)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
			}
			if f.calls != 1 || f.last != want {
				t.Errorf("searcher called %d times with %+v, want once with %+v", f.calls, f.last, want)
			}
		})
	}
}

func TestSearchEncodesResults(t *testing.T) {
	a := newAirports(t)
	f := newSearchFixture(t, 0, func(context.Context, flight.Query) ([]flight.Itinerary, error) {
		return []flight.Itinerary{directJFKLAX(t, a), oneStopJFKLAX(t, a)}, nil
	})
	rec := get(f.handler, "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}

	body := rec.Body.String()
	for _, fragment := range []string{
		`"currency":"USD"`,
		`"count":2`,
		`"date":"2024-03-15"`,
		`"layovers":[]`, // a direct itinerary, never null
		`"totalPrice":299.00`,
		`"price":189.00`,
		`"departureTime":"2024-03-15T08:30:00-04:00"`, // the time's own zone, not UTC
		`"arrivalTime":"2024-03-15T11:45:00-07:00"`,
		`"totalDurationMinutes":375`,
		`"durationMinutes":45`,
		`"type":"domestic"`,
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("body does not contain %s\n%s", fragment, body)
		}
	}
	if strings.Contains(body, "null") {
		t.Errorf("body contains null: %s", body)
	}
}

func TestSearchWithNoMatchesReturnsEmptyList(t *testing.T) {
	for name, result := range map[string][]flight.Itinerary{
		"nil slice":   nil,
		"empty slice": {},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSearchFixture(t, 0, func(context.Context, flight.Query) ([]flight.Itinerary, error) {
				return result, nil
			})
			rec := get(f.handler, "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-20")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if body := rec.Body.String(); !strings.Contains(body, `"itineraries":[]`) || !strings.Contains(body, `"count":0`) {
				t.Errorf("body = %s, want an empty itineraries array and a zero count", body)
			}
		})
	}
}

func TestSearchUnexpectedErrorIsHiddenButLogged(t *testing.T) {
	cause := fmt.Errorf("query shard 10.0.0.5 password=hunter2: %w", errors.New("connection refused"))
	f := newSearchFixture(t, 0, func(context.Context, flight.Query) ([]flight.Itinerary, error) {
		return nil, cause
	})
	rec := get(f.handler, "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15", "X-Request-ID", "req-500")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	for _, secret := range []string{"hunter2", "10.0.0.5", "connection refused", "shard"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("response leaks %q: %s", secret, rec.Body)
		}
	}
	want := problem{
		Type: "about:blank", Title: "Internal Server Error", Status: 500,
		Detail: "The request could not be completed. Please try again.",
		Code:   "internal_error", RequestID: "req-500",
	}
	if got := decodeProblem(t, rec.Body.Bytes()); !reflect.DeepEqual(got, want) {
		t.Errorf("problem = %+v, want %+v", got, want)
	}

	var logged map[string]any
	for _, rec := range f.logs() {
		if rec["level"] == "ERROR" {
			logged = rec
		}
	}
	if logged == nil {
		t.Fatal("no error-level log line for the failed search")
	}
	if logged["request_id"] != "req-500" {
		t.Errorf("logged request_id = %v, want req-500", logged["request_id"])
	}
	if msg, _ := logged["error"].(string); !strings.Contains(msg, "hunter2") {
		t.Errorf("logged error = %v, want the cause", logged["error"])
	}
}

func TestSearchErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{"invalid query", fmt.Errorf("unknown airport: %w", flight.ErrInvalidQuery), 400, "validation_failed", "The search request is invalid."},
		{"deadline exceeded", context.DeadlineExceeded, 503, "timeout", "The request took too long to complete. Please try again."},
		{"wrapped deadline exceeded", fmt.Errorf("search: %w", context.DeadlineExceeded), 503, "timeout", "The request took too long to complete. Please try again."},
		{"anything else", errors.New("boom"), 500, "internal_error", "The request could not be completed. Please try again."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newSearchFixture(t, 0, func(context.Context, flight.Query) ([]flight.Itinerary, error) {
				return nil, tc.err
			})
			rec := get(f.handler, "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15")

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			p := decodeProblem(t, rec.Body.Bytes())
			if p.Code != tc.wantCode || p.Detail != tc.wantDetail || p.Status != tc.wantStatus || p.Title != http.StatusText(tc.wantStatus) {
				t.Errorf("problem = %+v", p)
			}
		})
	}
}

func TestSearchExceedingRequestTimeoutReturns503(t *testing.T) {
	var sawDeadline bool
	f := newSearchFixture(t, 20*time.Millisecond, func(ctx context.Context, _ flight.Query) ([]flight.Itinerary, error) {
		_, sawDeadline = ctx.Deadline()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return nil, nil
		}
	})
	rec := get(f.handler, "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15", "X-Request-ID", "req-503")

	if !sawDeadline {
		t.Error("the searcher's context has no deadline")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	want := problem{
		Type: "about:blank", Title: "Service Unavailable", Status: 503,
		Detail: "The request took too long to complete. Please try again.",
		Code:   "timeout", RequestID: "req-503",
	}
	if got := decodeProblem(t, rec.Body.Bytes()); !reflect.DeepEqual(got, want) {
		t.Errorf("problem = %+v, want %+v", got, want)
	}
}

func TestPanicIsRecovered(t *testing.T) {
	calls := 0
	f := newSearchFixture(t, 0, func(context.Context, flight.Query) ([]flight.Itinerary, error) {
		calls++
		if calls == 1 {
			panic("index out of range: secret detail")
		}
		return nil, nil
	})
	const target = "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15"

	rec := get(f.handler, target, "X-Request-ID", "req-panic")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if strings.Contains(rec.Body.String(), "secret detail") {
		t.Errorf("response leaks the panic value: %s", rec.Body)
	}
	p := decodeProblem(t, rec.Body.Bytes())
	if p.Code != "internal_error" || p.RequestID != "req-panic" {
		t.Errorf("problem = %+v", p)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("the recovered response lost its Cache-Control header")
	}

	var logged map[string]any
	for _, r := range f.logs() {
		if r["msg"] == "panic recovered" {
			logged = r
		}
	}
	if logged == nil {
		t.Fatal("the panic was not logged")
	}
	if stack, _ := logged["stack"].(string); !strings.Contains(stack, "TestPanicIsRecovered") {
		t.Errorf("logged stack does not show the panicking code:\n%v", logged["stack"])
	}
	if logged["request_id"] != "req-panic" {
		t.Errorf("logged request_id = %v", logged["request_id"])
	}

	// The process survives: the next request is served normally.
	if next := get(f.handler, target); next.Code != http.StatusOK {
		t.Errorf("request after the panic: status = %d, want 200", next.Code)
	}
}
