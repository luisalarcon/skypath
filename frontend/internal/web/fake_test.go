package web

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/luisalarcon/skypath/frontend/internal/client"
)

// readExampleFile returns a canonical payload from api/examples.
func readExampleFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "examples", name))
	if err != nil {
		t.Fatalf("read example %s: %v", name, err)
	}
	return data
}

// loadExample decodes a canonical payload into v.
func loadExample(t *testing.T, name string, v any) {
	t.Helper()
	if err := json.Unmarshal(readExampleFile(t, name), v); err != nil {
		t.Fatalf("decode example %s: %v", name, err)
	}
}

// searchCall records the arguments of one Search call.
type searchCall struct {
	Origin, Destination, Date string
	RequestID                 string
}

// fakeAPI is a scriptable stand-in for the search API.
type fakeAPI struct {
	// SearchFunc answers Search. A nil func answers with an empty result.
	SearchFunc func(ctx context.Context, origin, destination, date string) (*client.SearchResponse, error)

	AirportList []client.Airport
	AirportsErr error
	DatasetInfo *client.Dataset
	DatasetErr  error

	mu            sync.Mutex
	searches      []searchCall
	airportsCalls int
	datasetCalls  int
}

// newFakeAPI returns a fake whose reference data is the canonical examples
// and whose searches return the given canonical search payload.
func newFakeAPI(t *testing.T, searchExample string) *fakeAPI {
	t.Helper()
	var airports struct {
		Airports []client.Airport `json:"airports"`
	}
	loadExample(t, "airports.json", &airports)
	var dataset client.Dataset
	loadExample(t, "dataset.json", &dataset)

	f := &fakeAPI{AirportList: airports.Airports, DatasetInfo: &dataset}
	if searchExample != "" {
		f.searchReturns(t, searchExample)
	}
	return f
}

// searchReturns makes every search answer with the named canonical payload.
func (f *fakeAPI) searchReturns(t *testing.T, example string) {
	t.Helper()
	var resp client.SearchResponse
	loadExample(t, example, &resp)
	f.SearchFunc = func(context.Context, string, string, string) (*client.SearchResponse, error) {
		return &resp, nil
	}
}

// failSearchWith makes every search fail with err.
func (f *fakeAPI) failSearchWith(err error) {
	f.SearchFunc = func(context.Context, string, string, string) (*client.SearchResponse, error) {
		return nil, err
	}
}

func (f *fakeAPI) Search(ctx context.Context, origin, destination, date string) (*client.SearchResponse, error) {
	f.mu.Lock()
	f.searches = append(f.searches, searchCall{origin, destination, date, client.RequestID(ctx)})
	f.mu.Unlock()
	if f.SearchFunc == nil {
		return &client.SearchResponse{Itineraries: []client.Itinerary{}}, nil
	}
	return f.SearchFunc(ctx, origin, destination, date)
}

func (f *fakeAPI) Airports(context.Context) ([]client.Airport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.airportsCalls++
	return f.AirportList, f.AirportsErr
}

func (f *fakeAPI) Dataset(context.Context) (*client.Dataset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.datasetCalls++
	return f.DatasetInfo, f.DatasetErr
}

// searchCalls returns a copy of the recorded Search calls.
func (f *fakeAPI) searchCalls() []searchCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]searchCall(nil), f.searches...)
}

// problemFrom decodes a canonical problem payload into the error the client
// would return for it.
func problemFrom(t *testing.T, name string) *client.ProblemError {
	t.Helper()
	var p client.ProblemError
	loadExample(t, name, &p)
	return &p
}
