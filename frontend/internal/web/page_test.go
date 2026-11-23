package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/view"
)

const testAssetVersion = "test-version"

func newTestServer(api API) *server {
	return newServer(api, discardLogger(), testAssetVersion)
}

// buildQuery parses a raw query string the way net/http does.
func buildQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query %q: %v", raw, err)
	}
	return q
}

func TestBuildPageInitialState(t *testing.T) {
	api := newFakeAPI(t, "search-success.json")
	s := newTestServer(api)

	page, status := s.buildPage(context.Background(), url.Values{})

	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if page.Title != "SkyPath · Flight search" || page.AssetVersion != testAssetVersion {
		t.Errorf("title %q, asset version %q", page.Title, page.AssetVersion)
	}
	if page.Results != nil || page.Notice != nil || page.Form.Errors.Any() {
		t.Errorf("initial page has results, a notice or errors: %+v", page)
	}
	if len(api.searchCalls()) != 0 {
		t.Errorf("an initial visit ran a search")
	}

	form := page.Form
	if form.Origin != "" || form.Destination != "" || form.OriginName != "" || form.DestinationName != "" {
		t.Errorf("form should be empty, got %+v", form)
	}
	if form.Date != "2024-03-15" || form.MinDate != "2024-03-15" || form.MaxDate != "2024-03-16" {
		t.Errorf("date defaults = %q within %q..%q, want the schedule's range", form.Date, form.MinDate, form.MaxDate)
	}

	wantDataset := view.Dataset{FirstDate: "Mar 15, 2024", LastDate: "Mar 16, 2024", FirstDateISO: "2024-03-15", LastDateISO: "2024-03-16"}
	if page.Dataset == nil || *page.Dataset != wantDataset {
		t.Errorf("dataset = %+v, want %+v", page.Dataset, wantDataset)
	}
	if len(page.Airports) != 3 || page.Airports[0] != (view.Airport{Code: "JFK", Name: "John F. Kennedy International", City: "New York", Country: "US"}) {
		t.Errorf("airports = %+v", page.Airports)
	}
}

func TestBuildPageUnrelatedParametersAreNotASearch(t *testing.T) {
	api := newFakeAPI(t, "search-success.json")
	s := newTestServer(api)

	page, status := s.buildPage(context.Background(), buildQuery(t, "utm_source=newsletter&ref=x"))
	if status != http.StatusOK || page.Results != nil || page.Form.Errors.Any() {
		t.Errorf("status %d, page %+v", status, page)
	}
	if len(api.searchCalls()) != 0 {
		t.Errorf("unrelated parameters triggered a search")
	}
	if page.Form.Date != "2024-03-15" {
		t.Errorf("date = %q, want the default", page.Form.Date)
	}
}

func TestBuildPageWithoutReferenceData(t *testing.T) {
	tests := []struct {
		name         string
		breakIt      func(*fakeAPI)
		wantAirports bool
		wantDataset  bool
	}{
		{"airports unavailable", func(f *fakeAPI) { f.AirportsErr = fmt.Errorf("%w: down", client.ErrUnavailable) }, false, true},
		{"dataset unavailable", func(f *fakeAPI) { f.DatasetErr = fmt.Errorf("%w: down", client.ErrUnavailable) }, true, false},
		{"both unavailable", func(f *fakeAPI) {
			f.AirportsErr = fmt.Errorf("%w: down", client.ErrUnavailable)
			f.DatasetErr = fmt.Errorf("%w: down", client.ErrUnavailable)
		}, false, false},
		{"dataset with unusable dates", func(f *fakeAPI) { f.DatasetInfo = &client.Dataset{FirstDepartureDate: "soon"} }, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, "search-success.json")
			tt.breakIt(api)
			s := newTestServer(api)

			t.Run("initial", func(t *testing.T) {
				page, status := s.buildPage(context.Background(), url.Values{})
				if status != http.StatusOK || page.Notice != nil {
					t.Errorf("status %d notice %+v; missing reference data must not be an error", status, page.Notice)
				}
				if (len(page.Airports) > 0) != tt.wantAirports || (page.Dataset != nil) != tt.wantDataset {
					t.Errorf("airports %d, dataset %+v", len(page.Airports), page.Dataset)
				}
				if !tt.wantDataset && (page.Form.Date != "" || page.Form.MinDate != "" || page.Form.MaxDate != "") {
					t.Errorf("date fields should be empty without a schedule range: %+v", page.Form)
				}
			})

			t.Run("search", func(t *testing.T) {
				page, status := s.buildPage(context.Background(), buildQuery(t, "origin=JFK&destination=LAX&date=2024-03-15"))
				if status != http.StatusOK || page.Results == nil || page.Notice != nil {
					t.Errorf("status %d results %v notice %+v", status, page.Results != nil, page.Notice)
				}
			})
		})
	}
}

func TestBuildPageSearchResults(t *testing.T) {
	tests := []struct {
		name        string
		example     string
		query       string
		wantTitle   string
		wantCount   int
		wantOrigin  string
		wantDestIn  string
		wantOrigNm  string
		wantDestNm  string
		wantFirstTg []string
	}{
		{
			name: "direct and one stop", example: "search-success.json",
			query:     "origin=JFK&destination=LAX&date=2024-03-15",
			wantTitle: "JFK to LAX · Mar 15, 2024 · SkyPath", wantCount: 2,
			wantOrigin: "JFK", wantDestIn: "LAX",
			wantOrigNm: "New York · John F. Kennedy International", wantDestNm: "Los Angeles · Los Angeles International",
			wantFirstTg: []string{"Fastest", "Cheapest"},
		},
		{
			name: "two stops to an airport the list does not know", example: "search-two-stops.json",
			query:     "origin=SFO&destination=NRT&date=2024-03-15",
			wantTitle: "SFO to NRT · Mar 15, 2024 · SkyPath", wantCount: 1,
			wantOrigin: "SFO", wantDestIn: "NRT",
			wantFirstTg: []string{"Fastest", "Cheapest"},
		},
		{
			name: "no itineraries", example: "search-empty.json",
			query:     "origin=JFK&destination=LAX&date=2024-03-20",
			wantTitle: "JFK to LAX · Mar 20, 2024 · SkyPath", wantCount: 0,
			wantOrigin: "JFK", wantDestIn: "LAX",
			wantOrigNm: "New York · John F. Kennedy International", wantDestNm: "Los Angeles · Los Angeles International",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.example)
			s := newTestServer(api)

			page, status := s.buildPage(client.WithRequestID(context.Background(), "rid-1"), buildQuery(t, tt.query))

			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			if page.Notice != nil || page.Form.Errors.Any() {
				t.Errorf("successful search has notice %+v or errors %+v", page.Notice, page.Form.Errors)
			}
			if page.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", page.Title, tt.wantTitle)
			}
			if page.Results == nil || page.Results.Count != tt.wantCount || len(page.Results.Itineraries) != tt.wantCount {
				t.Fatalf("results = %+v", page.Results)
			}
			if page.Results.Origin.Code != tt.wantOrigin || page.Results.Destination.Code != tt.wantDestIn {
				t.Errorf("results route = %s → %s", page.Results.Origin.Code, page.Results.Destination.Code)
			}
			if tt.wantCount > 0 {
				if got := strings.Join(page.Results.Itineraries[0].Tags, ","); got != strings.Join(tt.wantFirstTg, ",") {
					t.Errorf("first itinerary tags = %q, want %q", got, strings.Join(tt.wantFirstTg, ","))
				}
			}

			if page.Form.OriginName != tt.wantOrigNm || page.Form.DestinationName != tt.wantDestNm {
				t.Errorf("airport names = %q, %q; want %q, %q", page.Form.OriginName, page.Form.DestinationName, tt.wantOrigNm, tt.wantDestNm)
			}

			calls := api.searchCalls()
			want := searchCall{Origin: page.Form.Origin, Destination: page.Form.Destination, Date: page.Form.Date, RequestID: "rid-1"}
			if len(calls) != 1 || calls[0] != want {
				t.Errorf("search calls = %+v, want exactly %+v", calls, want)
			}
		})
	}
}

func TestBuildPageEchoesInputExactly(t *testing.T) {
	api := newFakeAPI(t, "search-success.json")
	s := newTestServer(api)

	page, status := s.buildPage(context.Background(), buildQuery(t, "origin=+jfk+&destination=lAx&date=2024-03-15"))

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if page.Form.Origin != " jfk " || page.Form.Destination != "lAx" || page.Form.Date != "2024-03-15" {
		t.Errorf("form = %+v, want the input untouched", page.Form)
	}
	// The hint still resolves, as the API accepts the same sloppy input.
	if page.Form.OriginName != "New York · John F. Kennedy International" || page.Form.DestinationName == "" {
		t.Errorf("hints = %q, %q", page.Form.OriginName, page.Form.DestinationName)
	}
	if got := api.searchCalls()[0]; got.Origin != " jfk " || got.Destination != "lAx" {
		t.Errorf("API received %+v; the frontend must not re-implement validation", got)
	}
}

func TestBuildPageSearchTriggers(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		origin string
		dest   string
		date   string
	}{
		{"only origin", "origin=JFK", "JFK", "", ""},
		{"only destination", "destination=LAX", "", "LAX", ""},
		{"only date", "date=2024-03-15", "", "", "2024-03-15"},
		{"all present but empty", "origin=&destination=&date=", "", "", ""},
		{"present without a value", "origin", "", "", ""},
		{"first of repeated values", "origin=JFK&origin=ORD&destination=LAX&date=2024-03-15", "JFK", "LAX", "2024-03-15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, "search-empty.json")
			s := newTestServer(api)

			page, _ := s.buildPage(context.Background(), buildQuery(t, tt.query))

			calls := api.searchCalls()
			if len(calls) != 1 {
				t.Fatalf("search calls = %d, want 1", len(calls))
			}
			if calls[0].Origin != tt.origin || calls[0].Destination != tt.dest || calls[0].Date != tt.date {
				t.Errorf("API received %+v", calls[0])
			}
			// A cleared date stays cleared; the default is only for a fresh form.
			if page.Form.Date != tt.date {
				t.Errorf("form date = %q, want %q", page.Form.Date, tt.date)
			}
		})
	}
}

func TestBuildPageInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		problem  *client.ProblemError
		wantForm view.FieldErrors
	}{
		{
			name:    "canonical validation problem",
			problem: nil, // filled from the example below
			wantForm: view.FieldErrors{
				Origin: "Unknown airport code XXX.",
				Date:   "Date must be a valid date in YYYY-MM-DD format.",
			},
		},
		{
			name: "every field missing",
			problem: &client.ProblemError{Status: 400, Code: "validation_failed", Errors: []client.FieldError{
				{Field: "origin", Code: "required", Message: "Origin is required."},
				{Field: "destination", Code: "required", Message: "Destination is required."},
				{Field: "date", Code: "required", Message: "Date is required."},
			}},
			wantForm: view.FieldErrors{Origin: "Origin is required.", Destination: "Destination is required.", Date: "Date is required."},
		},
		{
			name: "same airport is reported on the destination",
			problem: &client.ProblemError{Status: 400, Code: "validation_failed", Errors: []client.FieldError{
				{Field: "destination", Code: "same_as_origin", Message: "Destination must be different from origin."},
			}},
			wantForm: view.FieldErrors{Destination: "Destination must be different from origin."},
		},
		{
			name: "first message per field wins",
			problem: &client.ProblemError{Status: 400, Code: "validation_failed", Errors: []client.FieldError{
				{Field: "origin", Code: "invalid_format", Message: "first"},
				{Field: "origin", Code: "unknown_airport", Message: "second"},
			}},
			wantForm: view.FieldErrors{Origin: "first"},
		},
		{
			name: "unknown fields are ignored",
			problem: &client.ProblemError{Status: 400, Code: "validation_failed", Errors: []client.FieldError{
				{Field: "cabin", Code: "invalid_format", Message: "ignored"},
				{Field: "date", Code: "required", Message: "Date is required."},
			}},
			wantForm: view.FieldErrors{Date: "Date is required."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problem := tt.problem
			if problem == nil {
				problem = problemFrom(t, "problem-validation.json")
			}
			api := newFakeAPI(t, "")
			api.failSearchWith(problem)
			s := newTestServer(api)

			page, status := s.buildPage(context.Background(), buildQuery(t, "origin=XXX&destination=lax&date=soon"))

			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", status)
			}
			if page.Form.Errors != tt.wantForm {
				t.Errorf("field errors = %+v, want %+v", page.Form.Errors, tt.wantForm)
			}
			if page.Results != nil || page.Notice != nil {
				t.Errorf("invalid input should show neither results nor a banner: %+v / %+v", page.Results, page.Notice)
			}
			if page.Form.Origin != "XXX" || page.Form.Destination != "lax" || page.Form.Date != "soon" {
				t.Errorf("form lost the input: %+v", page.Form)
			}
			if page.Form.DestinationName != "Los Angeles · Los Angeles International" || page.Form.OriginName != "" {
				t.Errorf("hints = %q, %q; only known codes resolve", page.Form.OriginName, page.Form.DestinationName)
			}
			if page.Title != "SkyPath · Flight search" {
				t.Errorf("title = %q", page.Title)
			}
		})
	}
}

func TestBuildPageServiceFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantTitle  string
	}{
		{"unreachable", fmt.Errorf("%w: connection refused", client.ErrUnavailable), http.StatusBadGateway, "Search is temporarily unavailable"},
		{"backend error", fmt.Errorf("%w: unexpected status 500", client.ErrUnavailable), http.StatusBadGateway, "Search is temporarily unavailable"},
		{"timeout", fmt.Errorf("%w: %w: deadline", client.ErrUnavailable, client.ErrTimeout), http.StatusGatewayTimeout, "The search took too long"},
		{"problem that is not about the input", &client.ProblemError{Status: 404, Code: "not_found"}, http.StatusBadGateway, "Search is temporarily unavailable"},
		{"validation problem with nothing to show", &client.ProblemError{Status: 400, Code: "validation_failed"}, http.StatusBadGateway, "Search is temporarily unavailable"},
		{"unclassified error", errors.New("boom"), http.StatusBadGateway, "Search is temporarily unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, "")
			api.failSearchWith(tt.err)
			s := newTestServer(api)

			page, status := s.buildPage(context.Background(), buildQuery(t, "origin=JFK&destination=LAX&date=2024-03-15"))

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if page.Notice == nil || page.Notice.Kind != view.NoticeError || page.Notice.Title != tt.wantTitle || page.Notice.Message == "" {
				t.Fatalf("notice = %+v, want an error titled %q", page.Notice, tt.wantTitle)
			}
			if page.Results != nil || page.Form.Errors.Any() {
				t.Errorf("a service failure must not show results or field errors")
			}
			if page.Form.Origin != "JFK" || page.Form.Destination != "LAX" || page.Form.Date != "2024-03-15" {
				t.Errorf("form lost the search: %+v", page.Form)
			}
			if page.Form.OriginName == "" || len(page.Airports) == 0 || page.Dataset == nil {
				t.Errorf("the form should keep its hints and suggestions: %+v", page.Form)
			}
		})
	}
}

func TestBuildPageNoticeWordingIsPlain(t *testing.T) {
	for _, err := range []error{client.ErrUnavailable, client.ErrTimeout} {
		api := newFakeAPI(t, "")
		api.failSearchWith(fmt.Errorf("%w: %w", client.ErrUnavailable, err))
		page, _ := newTestServer(api).buildPage(context.Background(), buildQuery(t, "origin=JFK"))

		text := page.Notice.Title + " " + page.Notice.Message
		for _, jargon := range []string{"502", "504", "gateway", "backend", "API", "error", "HTTP"} {
			if strings.Contains(text, jargon) {
				t.Errorf("notice %q contains %q", text, jargon)
			}
		}
		if !strings.Contains(page.Notice.Message, "try again") {
			t.Errorf("notice %q does not invite a retry", text)
		}
	}
}

func TestBuildPageLogsServiceFailuresWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	api := newFakeAPI(t, "")
	api.failSearchWith(fmt.Errorf("%w: connection refused", client.ErrUnavailable))
	s := newServer(api, slog.New(slog.NewTextHandler(&buf, nil)), testAssetVersion)

	s.buildPage(client.WithRequestID(context.Background(), "rid-9"), buildQuery(t, "origin=JFK"))

	out := buf.String()
	for _, want := range []string{"level=ERROR", "search failed", "request_id=rid-9", "connection refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q does not contain %q", out, want)
		}
	}
}

func TestBuildPageDoesNotLogValidationFailures(t *testing.T) {
	var buf bytes.Buffer
	api := newFakeAPI(t, "")
	api.failSearchWith(problemFrom(t, "problem-validation.json"))
	s := newServer(api, slog.New(slog.NewTextHandler(&buf, nil)), testAssetVersion)

	s.buildPage(context.Background(), buildQuery(t, "origin=XXX"))

	if strings.Contains(buf.String(), "level=ERROR") {
		t.Errorf("a user's typo was logged as an error: %s", buf.String())
	}
}

func TestBuildPageFetchesReferenceDataOncePerTTL(t *testing.T) {
	api := newFakeAPI(t, "search-success.json")
	s := newTestServer(api)

	for i := 0; i < 5; i++ {
		s.buildPage(context.Background(), buildQuery(t, "origin=JFK&destination=LAX&date=2024-03-15"))
	}

	if api.airportsCalls != 1 || api.datasetCalls != 1 {
		t.Errorf("reference data fetched %d and %d times, want once each", api.airportsCalls, api.datasetCalls)
	}
	if n := len(api.searchCalls()); n != 5 {
		t.Errorf("searches = %d, want 5; searches must not be cached", n)
	}
}

func TestFieldErrorsOfNothing(t *testing.T) {
	if got := fieldErrors(nil); got.Any() {
		t.Errorf("fieldErrors(nil) = %+v, want none", got)
	}
}
