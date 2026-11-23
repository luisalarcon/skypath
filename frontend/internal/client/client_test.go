package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readExample loads a canonical payload from api/examples. The examples are
// the contract between the services, so tests use them rather than copies.
func readExample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "examples", name))
	if err != nil {
		t.Fatalf("read example %s: %v", name, err)
	}
	return data
}

// serveExample answers every request with the named example.
func serveExample(t *testing.T, status int, contentType, name string) http.Handler {
	t.Helper()
	body := readExample(t, name)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

// newTestClient starts a server running h and returns a client for it.
func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, srv.Client())
}

// assertNoUnknownFields fails if the example carries a field that target
// does not model, which would mean the client silently drops contract data.
func assertNoUnknownFields(t *testing.T, name string, target any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(readExample(t, name)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		t.Errorf("%s is not fully modelled by %T: %v", name, target, err)
	}
}

func TestExamplesAreFullyModelled(t *testing.T) {
	tests := []struct {
		file   string
		target any
	}{
		{"search-success.json", &SearchResponse{}},
		{"search-two-stops.json", &SearchResponse{}},
		{"search-empty.json", &SearchResponse{}},
		{"airports.json", &airportsResponse{}},
		{"dataset.json", &Dataset{}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assertNoUnknownFields(t, tt.file, tt.target)
		})
	}
}

func offsetSeconds(t time.Time) int {
	_, off := t.Zone()
	return off
}

func TestSearchDecodesSuccessExample(t *testing.T) {
	c := newTestClient(t, serveExample(t, 200, "application/json", "search-success.json"))

	got, err := c.Search(context.Background(), "JFK", "LAX", "2024-03-15")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if got.Origin.Code != "JFK" || got.Origin.Timezone != "America/New_York" || got.Destination.City != "Los Angeles" {
		t.Errorf("airports decoded wrongly: %+v / %+v", got.Origin, got.Destination)
	}
	if got.Date != "2024-03-15" || got.Currency != "USD" || got.Count != 2 || len(got.Itineraries) != 2 {
		t.Fatalf("header decoded wrongly: %+v", got)
	}

	direct, oneStop := got.Itineraries[0], got.Itineraries[1]
	if direct.Stops != 0 || direct.TotalDurationMinutes != 375 || direct.TotalPrice != 299 {
		t.Errorf("direct itinerary decoded wrongly: %+v", direct)
	}
	if offsetSeconds(direct.DepartureTime) != -4*3600 || offsetSeconds(direct.ArrivalTime) != -7*3600 {
		t.Errorf("direct itinerary lost its offsets: dep %v arr %v", direct.DepartureTime, direct.ArrivalTime)
	}
	if direct.Layovers == nil || len(direct.Layovers) != 0 {
		t.Errorf("direct layovers = %#v, want empty list", direct.Layovers)
	}

	if oneStop.Stops != 1 || len(oneStop.Segments) != 2 || len(oneStop.Layovers) != 1 {
		t.Fatalf("one-stop itinerary decoded wrongly: %+v", oneStop)
	}
	seg := oneStop.Segments[0]
	if seg.FlightNumber != "SP110" || seg.Airline != "SkyPath Airways" || seg.Aircraft != "A319" ||
		seg.Origin.Code != "JFK" || seg.Destination.Code != "ORD" ||
		seg.DurationMinutes != 150 || seg.Price != 189 {
		t.Errorf("segment decoded wrongly: %+v", seg)
	}
	if offsetSeconds(seg.ArrivalTime) != -5*3600 || seg.ArrivalTime.Hour() != 8 || seg.ArrivalTime.Minute() != 30 {
		t.Errorf("segment arrival = %v, want 08:30 at -05:00", seg.ArrivalTime)
	}
	lay := oneStop.Layovers[0]
	if lay.Airport.Code != "ORD" || lay.DurationMinutes != 45 || lay.Type != LayoverDomestic {
		t.Errorf("layover decoded wrongly: %+v", lay)
	}
}

func TestSearchDecodesTwoStopExample(t *testing.T) {
	c := newTestClient(t, serveExample(t, 200, "application/json", "search-two-stops.json"))

	got, err := c.Search(context.Background(), "SFO", "NRT", "2024-03-15")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got.Itineraries) != 1 {
		t.Fatalf("itineraries = %d, want 1", len(got.Itineraries))
	}
	it := got.Itineraries[0]
	if it.Stops != 2 || len(it.Segments) != 3 || len(it.Layovers) != 2 {
		t.Fatalf("itinerary shape wrong: %+v", it)
	}
	if it.TotalDurationMinutes != 1260 || it.TotalPrice != 1137 {
		t.Errorf("totals = %d min, %v; want 1260, 1137", it.TotalDurationMinutes, it.TotalPrice)
	}
	if offsetSeconds(it.ArrivalTime) != 9*3600 || it.ArrivalTime.Day() != 17 {
		t.Errorf("arrival = %v, want the 17th at +09:00", it.ArrivalTime)
	}
	if it.Layovers[0].Type != LayoverDomestic || it.Layovers[1].Type != LayoverInternational {
		t.Errorf("layover types = %q, %q", it.Layovers[0].Type, it.Layovers[1].Type)
	}
}

func TestSearchDecodesEmptyExample(t *testing.T) {
	c := newTestClient(t, serveExample(t, 200, "application/json", "search-empty.json"))

	got, err := c.Search(context.Background(), "JFK", "LAX", "2024-03-20")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if got.Count != 0 || got.Itineraries == nil || len(got.Itineraries) != 0 {
		t.Errorf("empty result decoded wrongly: count %d itineraries %#v", got.Count, got.Itineraries)
	}
}

func TestAirportsDecodesExample(t *testing.T) {
	c := newTestClient(t, serveExample(t, 200, "application/json", "airports.json"))

	got, err := c.Airports(context.Background())
	if err != nil {
		t.Fatalf("Airports returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("airports = %d, want 3", len(got))
	}
	if want := (Airport{Code: "ORD", Name: "O'Hare International", City: "Chicago", Country: "US", Timezone: "America/Chicago"}); got[2] != want {
		t.Errorf("third airport = %+v, want %+v", got[2], want)
	}
}

func TestDatasetDecodesExample(t *testing.T) {
	c := newTestClient(t, serveExample(t, 200, "application/json", "dataset.json"))

	got, err := c.Dataset(context.Background())
	if err != nil {
		t.Fatalf("Dataset returned error: %v", err)
	}
	want := Dataset{Airports: 25, Flights: 302, FirstDepartureDate: "2024-03-15", LastDepartureDate: "2024-03-16", SkippedRecords: 1}
	if *got != want {
		t.Errorf("dataset = %+v, want %+v", *got, want)
	}
}

func TestRequestShape(t *testing.T) {
	tests := []struct {
		name      string
		call      func(*Client, context.Context) error
		wantPath  string
		wantQuery map[string]string
	}{
		{
			name: "search sends the three values",
			call: func(c *Client, ctx context.Context) error {
				_, err := c.Search(ctx, "JFK", "LAX", "2024-03-15")
				return err
			},
			wantPath:  "/api/v1/search",
			wantQuery: map[string]string{"origin": "JFK", "destination": "LAX", "date": "2024-03-15"},
		},
		{
			name: "search escapes reserved characters",
			call: func(c *Client, ctx context.Context) error {
				_, err := c.Search(ctx, "J&K=1", " L X ", "2024-03-15#x?y")
				return err
			},
			wantPath:  "/api/v1/search",
			wantQuery: map[string]string{"origin": "J&K=1", "destination": " L X ", "date": "2024-03-15#x?y"},
		},
		{
			name: "search escapes non-ASCII text",
			call: func(c *Client, ctx context.Context) error {
				_, err := c.Search(ctx, "zürich", "東京", "")
				return err
			},
			wantPath:  "/api/v1/search",
			wantQuery: map[string]string{"origin": "zürich", "destination": "東京", "date": ""},
		},
		{
			name: "airports has no query",
			call: func(c *Client, ctx context.Context) error {
				_, err := c.Airports(ctx)
				return err
			},
			wantPath: "/api/v1/airports",
		},
		{
			name: "dataset has no query",
			call: func(c *Client, ctx context.Context) error {
				_, err := c.Dataset(ctx)
				return err
			},
			wantPath: "/api/v1/dataset",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *http.Request
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r
				// An empty object decodes into every response type.
				_, _ = w.Write([]byte(`{}`))
			}))

			if err := tt.call(c, WithRequestID(context.Background(), "req-123")); err != nil {
				t.Fatalf("call returned error: %v", err)
			}

			if got.Method != http.MethodGet || got.URL.Path != tt.wantPath {
				t.Errorf("request = %s %s, want GET %s", got.Method, got.URL.Path, tt.wantPath)
			}
			if h := got.Header.Get("Accept"); h != "application/json" {
				t.Errorf("Accept = %q, want application/json", h)
			}
			if h := got.Header.Get("X-Request-ID"); h != "req-123" {
				t.Errorf("X-Request-ID = %q, want req-123", h)
			}
			query := got.URL.Query()
			if len(query) != len(tt.wantQuery) {
				t.Errorf("query = %v, want %v", query, tt.wantQuery)
			}
			for k, want := range tt.wantQuery {
				if got := query.Get(k); got != want {
					t.Errorf("query %s = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestRequestIDHeaderOmittedWithoutID(t *testing.T) {
	var header http.Header
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header
		_, _ = w.Write([]byte(`{}`))
	}))
	if _, err := c.Dataset(context.Background()); err != nil {
		t.Fatalf("Dataset returned error: %v", err)
	}
	if _, ok := header["X-Request-Id"]; ok {
		t.Errorf("X-Request-ID was sent without an ID in the context")
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL+"/", nil).Dataset(context.Background()); err != nil {
		t.Fatalf("Dataset returned error: %v", err)
	}
	if path != "/api/v1/dataset" {
		t.Errorf("path = %q, want /api/v1/dataset", path)
	}
}

func TestFailuresWrapErrUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.Handler
		searchOnly bool // the body is only wrong for the search response's shape
	}{
		{"server error", serveExample(t, 500, "application/problem+json", "problem-internal.json"), false},
		{"malformed body", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"origin": `))
		}), false},
		{"wrong shape", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"itineraries": "none"}`))
		}), true},
		{"invalid timestamp", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"itineraries":[{"departureTime":"tomorrow"}]}`))
		}), true},
		{"empty body", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), false},
		{"html error page", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<h1>Bad gateway</h1>"))
		}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, tt.handler)
			if _, err := c.Search(context.Background(), "JFK", "LAX", "2024-03-15"); !errors.Is(err, ErrUnavailable) {
				t.Errorf("Search error = %v, want ErrUnavailable", err)
			}
			if tt.searchOnly {
				return
			}
			if _, err := c.Airports(context.Background()); !errors.Is(err, ErrUnavailable) {
				t.Errorf("Airports error = %v, want ErrUnavailable", err)
			}
			if _, err := c.Dataset(context.Background()); !errors.Is(err, ErrUnavailable) {
				t.Errorf("Dataset error = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestUnreachableServerWrapsErrUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	c := New(srv.URL, srv.Client())
	srv.Close()

	_, err := c.Dataset(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("error = %v, want ErrUnavailable", err)
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"airports": 1}` + strings.Repeat(" ", 200)))
	}))
	c.maxBytes = 100

	_, err := c.Dataset(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error %q does not explain the size limit", err)
	}
}

func TestBodyAtTheLimitIsAccepted(t *testing.T) {
	const doc = `{"airports": 1}`
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(doc))
	}))
	c.maxBytes = int64(len(doc))

	got, err := c.Dataset(context.Background())
	if err != nil {
		t.Fatalf("Dataset returned error: %v", err)
	}
	if got.Airports != 1 {
		t.Errorf("Airports = %d, want 1", got.Airports)
	}
}

func TestCancelledContextWrapsErrUnavailable(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Dataset(ctx)
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want ErrUnavailable wrapping context.Canceled", err)
	}
}
