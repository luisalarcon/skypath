package api

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func newReferenceHandler(t *testing.T, dir stubDirectory, ds DatasetInfo) http.Handler {
	t.Helper()
	logger, _ := testLogger()
	return NewHandler(Dependencies{Airports: dir, Dataset: ds, Logger: logger})
}

func TestAirports(t *testing.T) {
	a := newAirports(t)
	h := newReferenceHandler(t, stubDirectory{a.JFK, a.LAX, a.ORD}, sampleDataset())

	rec := get(h, "/api/v1/airports")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	want := map[string]any{
		"count": float64(3),
		"airports": []any{
			map[string]any{"code": "JFK", "name": "John F. Kennedy International", "city": "New York", "country": "US", "timezone": "America/New_York"},
			map[string]any{"code": "LAX", "name": "Los Angeles International", "city": "Los Angeles", "country": "US", "timezone": "America/Los_Angeles"},
			map[string]any{"code": "ORD", "name": "O'Hare International", "city": "Chicago", "country": "US", "timezone": "America/Chicago"},
		},
	}
	if got := decodeBody(t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

func TestAirportsEmptyListIsAnArray(t *testing.T) {
	rec := get(newReferenceHandler(t, nil, sampleDataset()), "/api/v1/airports")

	if body := rec.Body.String(); !strings.Contains(body, `"airports":[]`) || !strings.Contains(body, `"count":0`) {
		t.Errorf("body = %s, want an empty array and a zero count", body)
	}
}

func TestDataset(t *testing.T) {
	rec := get(newReferenceHandler(t, nil, sampleDataset()), "/api/v1/dataset")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := map[string]any{
		"airports":           float64(25),
		"flights":            float64(302),
		"firstDepartureDate": "2024-03-15",
		"lastDepartureDate":  "2024-03-16",
		"skippedRecords":     float64(1),
	}
	if got := decodeBody(t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

func TestReadyz(t *testing.T) {
	rec := get(newReferenceHandler(t, nil, sampleDataset()), "/readyz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := map[string]any{
		"status": "ready",
		"dataset": map[string]any{
			"airports": float64(25),
			"flights":  float64(302),
			"skipped": []any{
				map[string]any{"record": "flight SP995", "reason": `unknown origin airport "JKF"`},
			},
		},
	}
	if got := decodeBody(t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

func TestReadyzWithNothingSkippedHasEmptyArray(t *testing.T) {
	ds := sampleDataset()
	ds.Skipped = nil
	rec := get(newReferenceHandler(t, nil, ds), "/readyz")

	if body := rec.Body.String(); !strings.Contains(body, `"skipped":[]`) {
		t.Errorf("body = %s, want skipped to be an empty array, not null", body)
	}
}

func TestReferenceEndpointsRejectOtherMethods(t *testing.T) {
	h := newReferenceHandler(t, nil, sampleDataset())
	for _, path := range []string{"/api/v1/airports", "/api/v1/dataset", "/readyz"} {
		rec := do(h, http.MethodPost, path)
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("POST %s = %d, Allow %q; want 405, %q", path, rec.Code, rec.Header().Get("Allow"), "GET, HEAD")
		}
	}
}
