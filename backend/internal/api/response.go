package api

import (
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

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

// currency is the currency of every price. The schedule carries no currency
// of its own.
const currency = "USD"

type searchResponse struct {
	Origin      airportJSON     `json:"origin"`
	Destination airportJSON     `json:"destination"`
	Date        string          `json:"date"`
	Currency    string          `json:"currency"`
	Count       int             `json:"count"`
	Itineraries []itineraryJSON `json:"itineraries"`
}

type itineraryJSON struct {
	Stops                int           `json:"stops"`
	DepartureTime        string        `json:"departureTime"`
	ArrivalTime          string        `json:"arrivalTime"`
	TotalDurationMinutes int           `json:"totalDurationMinutes"`
	TotalPrice           flight.Money  `json:"totalPrice"`
	Segments             []segmentJSON `json:"segments"`
	Layovers             []layoverJSON `json:"layovers"`
}

type segmentJSON struct {
	FlightNumber    string       `json:"flightNumber"`
	Airline         string       `json:"airline"`
	Aircraft        string       `json:"aircraft"`
	Origin          airportJSON  `json:"origin"`
	Destination     airportJSON  `json:"destination"`
	DepartureTime   string       `json:"departureTime"`
	ArrivalTime     string       `json:"arrivalTime"`
	DurationMinutes int          `json:"durationMinutes"`
	Price           flight.Money `json:"price"`
}

type layoverJSON struct {
	Airport         airportJSON `json:"airport"`
	DurationMinutes int         `json:"durationMinutes"`
	Type            string      `json:"type"`
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

// newSearchResponse is the one place that turns a search result into its wire
// format.
func newSearchResponse(origin, destination flight.Airport, date flight.Date, itineraries []flight.Itinerary) searchResponse {
	// Non-nil so a search with no matches encodes as [] and not null.
	list := make([]itineraryJSON, 0, len(itineraries))
	for _, it := range itineraries {
		list = append(list, newItineraryJSON(it))
	}
	return searchResponse{
		Origin:      newAirportJSON(origin),
		Destination: newAirportJSON(destination),
		Date:        date.String(),
		Currency:    currency,
		Count:       len(list),
		Itineraries: list,
	}
}

func newItineraryJSON(it flight.Itinerary) itineraryJSON {
	segments := make([]segmentJSON, 0, len(it.Segments))
	for _, f := range it.Segments {
		segments = append(segments, segmentJSON{
			FlightNumber:    f.Number,
			Airline:         f.Airline,
			Aircraft:        f.Aircraft,
			Origin:          newAirportJSON(f.Origin),
			Destination:     newAirportJSON(f.Destination),
			DepartureTime:   formatTime(f.Departure),
			ArrivalTime:     formatTime(f.Arrival),
			DurationMinutes: wholeMinutes(f.Duration()),
			Price:           f.Price,
		})
	}
	// Non-nil so a direct itinerary encodes "layovers": [] and not null.
	layovers := make([]layoverJSON, 0, len(it.Layovers))
	for _, l := range it.Layovers {
		layovers = append(layovers, layoverJSON{
			Airport:         newAirportJSON(l.Airport),
			DurationMinutes: wholeMinutes(l.Duration),
			Type:            string(l.Kind),
		})
	}
	return itineraryJSON{
		Stops:                it.Stops(),
		DepartureTime:        formatTime(it.Departure()),
		ArrivalTime:          formatTime(it.Arrival()),
		TotalDurationMinutes: wholeMinutes(it.Duration()),
		TotalPrice:           it.Price(),
		Segments:             segments,
		Layovers:             layovers,
	}
}

// formatTime renders t as RFC 3339 in t's own zone. The flight data attaches
// each airport's zone to its times, so this yields the local wall-clock time
// and the UTC offset in one value.
func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func wholeMinutes(d time.Duration) int {
	return int(d / time.Minute)
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
