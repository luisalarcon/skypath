package api

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// examplesDir holds the canonical payloads shared with the API clients.
var examplesDir = filepath.Join("..", "..", "..", "api", "examples")

const (
	jsonType    = "application/json; charset=utf-8"
	problemType = "application/problem+json; charset=utf-8"

	// exampleRequestID is the requestId carried by both problem examples.
	exampleRequestID = "3f2a9c1e7b4d4a10"
)

// contractCase drives the handler with stub dependencies that produce one of
// the example files.
type contractCase struct {
	file        string
	target      string
	status      int
	contentType string
	requestID   string
	deps        func(t *testing.T, a airports) Dependencies
}

// searching returns dependencies whose searcher yields the given result.
func searching(result func(t *testing.T, a airports) ([]flight.Itinerary, error)) func(t *testing.T, a airports) Dependencies {
	return func(t *testing.T, a airports) Dependencies {
		return Dependencies{
			Airports: stubDirectory{a.DEN, a.JFK, a.LAX, a.NRT, a.ORD, a.SFO},
			Search: searcherFunc(func(context.Context, flight.Query) ([]flight.Itinerary, error) {
				return result(t, a)
			}),
		}
	}
}

func contractCases() []contractCase {
	return []contractCase{
		{
			file:        "search-success.json",
			target:      "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15",
			status:      200,
			contentType: jsonType,
			deps: searching(func(t *testing.T, a airports) ([]flight.Itinerary, error) {
				return []flight.Itinerary{directJFKLAX(t, a), oneStopJFKLAX(t, a)}, nil
			}),
		},
		{
			file:        "search-two-stops.json",
			target:      "/api/v1/search?origin=SFO&destination=NRT&date=2024-03-15",
			status:      200,
			contentType: jsonType,
			deps: searching(func(t *testing.T, a airports) ([]flight.Itinerary, error) {
				return []flight.Itinerary{twoStopSFONRT(t, a)}, nil
			}),
		},
		{
			file:        "search-empty.json",
			target:      "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-20",
			status:      200,
			contentType: jsonType,
			deps: searching(func(*testing.T, airports) ([]flight.Itinerary, error) {
				return []flight.Itinerary{}, nil
			}),
		},
		{
			file:        "problem-validation.json",
			target:      "/api/v1/search?origin=XXX&destination=LAX&date=15-03-2024",
			status:      400,
			contentType: problemType,
			requestID:   exampleRequestID,
			deps: searching(func(*testing.T, airports) ([]flight.Itinerary, error) {
				return nil, errors.New("the searcher must not be reached")
			}),
		},
		{
			file:        "problem-internal.json",
			target:      "/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15",
			status:      500,
			contentType: problemType,
			requestID:   exampleRequestID,
			deps: searching(func(*testing.T, airports) ([]flight.Itinerary, error) {
				return nil, errors.New("index is corrupt")
			}),
		},
		{
			file:        "airports.json",
			target:      "/api/v1/airports",
			status:      200,
			contentType: jsonType,
			deps: func(t *testing.T, a airports) Dependencies {
				return Dependencies{Airports: stubDirectory{a.JFK, a.LAX, a.ORD}}
			},
		},
		{
			file:        "dataset.json",
			target:      "/api/v1/dataset",
			status:      200,
			contentType: jsonType,
			deps:        func(*testing.T, airports) Dependencies { return Dependencies{Dataset: sampleDataset()} },
		},
		{
			file:        "ready.json",
			target:      "/readyz",
			status:      200,
			contentType: jsonType,
			deps:        func(*testing.T, airports) Dependencies { return Dependencies{Dataset: sampleDataset()} },
		},
	}
}

// TestContractExamples asserts that the handler reproduces every file in
// api/examples exactly, compared as decoded JSON.
func TestContractExamples(t *testing.T) {
	a := newAirports(t)
	for _, tc := range contractCases() {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(examplesDir, tc.file))
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			var want any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("example is not valid JSON: %v", err)
			}

			deps := tc.deps(t, a)
			deps.Logger, _ = testLogger()
			var headers []string
			if tc.requestID != "" {
				headers = []string{"X-Request-ID", tc.requestID}
			}
			rec := get(NewHandler(deps), tc.target, headers...)

			if rec.Code != tc.status {
				t.Errorf("status = %d, want %d", rec.Code, tc.status)
			}
			if got := rec.Header().Get("Content-Type"); got != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tc.contentType)
			}
			var got any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("response is not valid JSON: %q: %v", rec.Body.String(), err)
			}
			if !reflect.DeepEqual(got, want) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("response differs from %s\n got: %s\nwant: %s", tc.file, gotJSON, wantJSON)
			}
		})
	}
}

// TestContractCoversEveryExample fails when a file is added to api/examples
// without a case above, so a new payload cannot go unchecked.
func TestContractCoversEveryExample(t *testing.T) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read examples directory: %v", err)
	}
	var onDisk, covered []string
	for _, e := range entries {
		onDisk = append(onDisk, e.Name())
	}
	for _, tc := range contractCases() {
		covered = append(covered, tc.file)
	}
	sort.Strings(onDisk)
	sort.Strings(covered)
	if !reflect.DeepEqual(onDisk, covered) {
		t.Errorf("examples on disk %v\n  differ from those under test %v", onDisk, covered)
	}
}
