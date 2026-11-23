package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/view"
)

// API is what the handlers need from the search API. *client.Client
// satisfies it; tests supply a fake.
type API interface {
	Search(ctx context.Context, origin, destination, date string) (*client.SearchResponse, error)
	Airports(ctx context.Context) ([]client.Airport, error)
	Dataset(ctx context.Context) (*client.Dataset, error)
}

var _ API = (*client.Client)(nil)

const (
	// referenceTTL is how long the airport list and schedule range are kept.
	// Both change only when the backend is redeployed, so a minute is ample
	// freshness while keeping them off the path of nearly every request.
	referenceTTL = time.Minute
	// referenceRetry is how long a failed fetch is remembered. It is short so
	// the page recovers soon after the backend does.
	referenceRetry = 10 * time.Second

	titleInitial = "SkyPath · Flight search"

	// codeValidationFailed is the API's identifier for field-level problems.
	codeValidationFailed = "validation_failed"
)

// server holds what the handlers share. Everything in it is safe for
// concurrent use.
type server struct {
	api          API
	logger       *slog.Logger
	assetVersion string

	airports *cache[*airportDirectory]
	dataset  *cache[*view.Dataset]
}

func newServer(api API, logger *slog.Logger, assetVersion string) *server {
	s := &server{api: api, logger: logger, assetVersion: assetVersion}
	s.airports = newCache("airports", referenceTTL, referenceRetry, logger, s.loadAirports)
	s.dataset = newCache("dataset", referenceTTL, referenceRetry, logger, s.loadDataset)
	return s
}

// airportDirectory is the airport list in the two shapes the page needs: in
// order, for the suggestions, and by code, to name what the user typed.
type airportDirectory struct {
	list   []view.Airport
	byCode map[string]client.Airport
}

// describe returns "New York · John F. Kennedy International" for a code the
// schedule knows, or "" otherwise. It accepts the sloppy input the API also
// accepts: surrounding spaces and any letter case.
func (d *airportDirectory) describe(code string) string {
	if d == nil {
		return ""
	}
	a, ok := d.byCode[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return ""
	}
	return a.City + " · " + a.Name
}

func (s *server) loadAirports(ctx context.Context) (*airportDirectory, error) {
	airports, err := s.api.Airports(ctx)
	if err != nil {
		return nil, err
	}
	dir := &airportDirectory{
		list:   make([]view.Airport, 0, len(airports)),
		byCode: make(map[string]client.Airport, len(airports)),
	}
	for _, a := range airports {
		dir.list = append(dir.list, airportView(a))
		dir.byCode[a.Code] = a
	}
	return dir, nil
}

func (s *server) loadDataset(ctx context.Context) (*view.Dataset, error) {
	d, err := s.api.Dataset(ctx)
	if err != nil {
		return nil, err
	}
	first, err := time.Parse(isoDateLayout, d.FirstDepartureDate)
	if err != nil {
		return nil, fmt.Errorf("first departure date: %w", err)
	}
	last, err := time.Parse(isoDateLayout, d.LastDepartureDate)
	if err != nil {
		return nil, fmt.Errorf("last departure date: %w", err)
	}
	return &view.Dataset{
		FirstDate:    formatScheduleDate(first),
		LastDate:     formatScheduleDate(last),
		FirstDateISO: d.FirstDepartureDate,
		LastDateISO:  d.LastDepartureDate,
	}, nil
}

// isSearch reports whether the query asks for a search. Presence is what
// counts, not content: "?origin=&destination=&date=" is a submitted, empty
// form and deserves the API's "required" messages, while a bare "/" or a
// query with only unrelated parameters, such as campaign tags, is a visit.
func isSearch(q url.Values) bool {
	return q.Has("origin") || q.Has("destination") || q.Has("date")
}

// buildPage decides what the search page shows for the given query string and
// with which status. It performs the API calls but writes nothing, so the
// decision can be tested without rendering.
func (s *server) buildPage(ctx context.Context, q url.Values) (view.Page, int) {
	// Reference data only enriches the page. When it cannot be fetched the
	// page is rendered without it and is not treated as an error.
	directory, _ := s.airports.get(ctx)
	dataset, _ := s.dataset.get(ctx)

	page := view.Page{Title: titleInitial, AssetVersion: s.assetVersion, Dataset: dataset}
	if directory != nil {
		page.Airports = directory.list
	}
	if dataset != nil {
		page.Form.MinDate = dataset.FirstDateISO
		page.Form.MaxDate = dataset.LastDateISO
	}

	if !isSearch(q) {
		if dataset != nil {
			page.Form.Date = dataset.FirstDateISO
		}
		return page, http.StatusOK
	}

	// The form echoes exactly what was typed. Only the first value of a
	// repeated parameter is used, as the API does.
	page.Form.Origin = q.Get("origin")
	page.Form.Destination = q.Get("destination")
	page.Form.Date = q.Get("date")
	page.Form.OriginName = directory.describe(page.Form.Origin)
	page.Form.DestinationName = directory.describe(page.Form.Destination)

	status := s.search(ctx, &page)
	return page, status
}

// search runs the search described by the page's form and records the
// outcome on the page: results, field errors or a notice.
func (s *server) search(ctx context.Context, page *view.Page) int {
	form := &page.Form
	resp, err := s.api.Search(ctx, form.Origin, form.Destination, form.Date)
	if err == nil {
		page.Results = resultsView(resp)
		page.Title = resultsTitle(resp)
		return http.StatusOK
	}

	var problem *client.ProblemError
	if errors.As(err, &problem) && problem.Code == codeValidationFailed {
		if form.Errors = fieldErrors(problem.Errors); form.Errors.Any() {
			return http.StatusBadRequest
		}
	}

	// Anything else means the search could not be completed. A problem the
	// frontend could not map is included: it is not something the user can
	// fix by editing the form.
	s.logSearchFailure(ctx, err)
	if errors.Is(err, client.ErrTimeout) {
		page.Notice = &view.Notice{
			Kind:    view.NoticeError,
			Title:   "The search took too long",
			Message: "The flight search is responding slowly. Your search is still filled in below, so you can try again in a moment.",
		}
		return http.StatusGatewayTimeout
	}
	page.Notice = &view.Notice{
		Kind:    view.NoticeError,
		Title:   "Search is temporarily unavailable",
		Message: "We couldn't get flight results right now. Your search is still filled in below, so you can try again in a moment.",
	}
	return http.StatusBadGateway
}

// logSearchFailure records why a search failed. A visitor who closed the page
// mid-search is not a failure of ours, so it is logged quietly.
func (s *server) logSearchFailure(ctx context.Context, err error) {
	level := slog.LevelError
	if errors.Is(err, context.Canceled) {
		level = slog.LevelInfo
	}
	s.logger.Log(ctx, level, "search failed",
		"request_id", client.RequestID(ctx),
		"error", err)
}

// fieldErrors maps the API's per-field messages onto the form. Messages are
// written for end users, so they are shown as they are. When a field has
// several problems the first is kept: the API orders them by importance.
func fieldErrors(problems []client.FieldError) view.FieldErrors {
	var out view.FieldErrors
	set := func(dst *string, message string) {
		if *dst == "" {
			*dst = message
		}
	}
	for _, p := range problems {
		switch p.Field {
		case "origin":
			set(&out.Origin, p.Message)
		case "destination":
			set(&out.Destination, p.Message)
		case "date":
			set(&out.Date, p.Message)
		}
	}
	return out
}

// resultsTitle is "JFK to LAX · Mar 15, 2024 · SkyPath".
func resultsTitle(resp *client.SearchResponse) string {
	date := resp.Date
	if parsed, err := time.Parse(isoDateLayout, resp.Date); err == nil {
		date = formatScheduleDate(parsed)
	}
	return fmt.Sprintf("%s to %s · %s · SkyPath", resp.Origin.Code, resp.Destination.Code, date)
}
