package api

import "github.com/luisalarcon/skypath/backend/internal/flight"

// This file defines the JSON shapes of every successful response. They are
// separate from the domain types so the wire format, which clients depend on,
// can only change deliberately, and the domain can change without it.

type airportJSON struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Country  string `json:"country"`
	Timezone string `json:"timezone"`
}

type airportsResponse struct {
	Count    int           `json:"count"`
	Airports []airportJSON `json:"airports"`
}

type datasetResponse struct {
	Airports           int    `json:"airports"`
	Flights            int    `json:"flights"`
	FirstDepartureDate string `json:"firstDepartureDate"`
	LastDepartureDate  string `json:"lastDepartureDate"`
	SkippedRecords     int    `json:"skippedRecords"`
}

type readinessResponse struct {
	Status  string           `json:"status"`
	Dataset readinessDataset `json:"dataset"`
}

type readinessDataset struct {
	Airports int                 `json:"airports"`
	Flights  int                 `json:"flights"`
	Skipped  []skippedRecordJSON `json:"skipped"`
}

type skippedRecordJSON struct {
	Record string `json:"record"`
	Reason string `json:"reason"`
}

func newAirportJSON(a flight.Airport) airportJSON {
	return airportJSON{
		Code:     a.Code,
		Name:     a.Name,
		City:     a.City,
		Country:  a.Country,
		Timezone: a.Location.String(),
	}
}

func newAirportsResponse(airports []flight.Airport) airportsResponse {
	// Non-nil so an empty list encodes as [] and not null.
	list := make([]airportJSON, 0, len(airports))
	for _, a := range airports {
		list = append(list, newAirportJSON(a))
	}
	return airportsResponse{Count: len(list), Airports: list}
}

func newDatasetResponse(d DatasetInfo) datasetResponse {
	return datasetResponse{
		Airports:           d.Airports,
		Flights:            d.Flights,
		FirstDepartureDate: d.FirstDeparture.String(),
		LastDepartureDate:  d.LastDeparture.String(),
		SkippedRecords:     len(d.Skipped),
	}
}

func newReadinessResponse(d DatasetInfo) readinessResponse {
	skipped := make([]skippedRecordJSON, 0, len(d.Skipped))
	for _, s := range d.Skipped {
		skipped = append(skipped, skippedRecordJSON{Record: s.Record, Reason: s.Reason})
	}
	return readinessResponse{
		Status: "ready",
		Dataset: readinessDataset{
			Airports: d.Airports,
			Flights:  d.Flights,
			Skipped:  skipped,
		},
	}
}
