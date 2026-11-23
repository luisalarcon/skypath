package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/luisalarcon/skypath/backend/internal/api"
	"github.com/luisalarcon/skypath/backend/internal/config"
	"github.com/luisalarcon/skypath/backend/internal/dataset"
	"github.com/luisalarcon/skypath/backend/internal/flight"
	"github.com/luisalarcon/skypath/backend/internal/search"
	"github.com/luisalarcon/skypath/backend/internal/store"
)

// This is the only package that knows every other one, so it is where the
// interfaces each layer declares are checked against what implements them.
var (
	_ search.FlightSource  = (*store.Store)(nil)
	_ api.AirportDirectory = (*store.Store)(nil)
	_ api.Searcher         = (*search.Engine)(nil)
)

// newHandler loads the schedule and wires the service together: the loader
// feeds the store, the search engine reads the store, and the HTTP layer
// talks to the engine. It fails if the schedule cannot be used.
func newHandler(cfg config.Config, logger *slog.Logger) (http.Handler, error) {
	data, err := dataset.Load(cfg.FlightsPath)
	if err != nil {
		return nil, fmt.Errorf("load schedule: %w", err)
	}
	logLoadSummary(logger, cfg.FlightsPath, data)

	flights := store.New(data.Airports, data.Flights)

	return api.NewHandler(api.Dependencies{
		Search:         search.NewEngine(flights),
		Airports:       flights,
		Dataset:        describe(data),
		Logger:         logger,
		RequestTimeout: cfg.RequestTimeout,
	}), nil
}

// describe summarises the loaded schedule for the dataset and readiness
// endpoints.
func describe(data *dataset.Dataset) api.DatasetInfo {
	info := api.DatasetInfo{
		Airports: len(data.Airports),
		Flights:  len(data.Flights),
		Skipped:  make([]api.SkippedRecord, 0, len(data.Skipped)),
	}
	for i, f := range data.Flights {
		// A departure carries its origin's zone, so this is the local date a
		// traveller would search for.
		date := flight.DateOf(f.Departure)
		if i == 0 || date.Compare(info.FirstDeparture) < 0 {
			info.FirstDeparture = date
		}
		if i == 0 || date.Compare(info.LastDeparture) > 0 {
			info.LastDeparture = date
		}
	}
	for _, skipped := range data.Skipped {
		info.Skipped = append(info.Skipped, api.SkippedRecord(skipped))
	}
	return info
}

// logLoadSummary records what was loaded and every record that was not. A
// skipped record is a warning: the service is healthy, but someone should
// look at the source data.
func logLoadSummary(logger *slog.Logger, path string, data *dataset.Dataset) {
	logger.Info("schedule loaded",
		"path", path,
		"airports", len(data.Airports),
		"flights", len(data.Flights),
		"skipped", len(data.Skipped),
	)
	for _, skipped := range data.Skipped {
		logger.Warn("schedule record skipped", "record", skipped.Record, "reason", skipped.Reason)
	}
}
