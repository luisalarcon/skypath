package client

import "time"

// Airport is an airport as the API describes it.
type Airport struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Country  string `json:"country"`
	Timezone string `json:"timezone"`
}

// SearchResponse is the result of a search. Itineraries is empty, not nil,
// when the search was valid but matched nothing.
type SearchResponse struct {
	Origin      Airport     `json:"origin"`
	Destination Airport     `json:"destination"`
	Date        string      `json:"date"` // YYYY-MM-DD, the departure date at the origin
	Currency    string      `json:"currency"`
	Count       int         `json:"count"`
	Itineraries []Itinerary `json:"itineraries"`
}

// Itinerary is one way of travelling from origin to destination.
//
// Times are decoded from RFC 3339 and keep the UTC offset the API sent, so
// formatting one yields the local time at its airport. Layovers[i] is the
// connection between Segments[i] and Segments[i+1].
type Itinerary struct {
	Stops                int       `json:"stops"`
	DepartureTime        time.Time `json:"departureTime"`
	ArrivalTime          time.Time `json:"arrivalTime"`
	TotalDurationMinutes int       `json:"totalDurationMinutes"`
	TotalPrice           float64   `json:"totalPrice"`
	Segments             []Segment `json:"segments"`
	Layovers             []Layover `json:"layovers"`
}

// Segment is a single flight within an itinerary.
type Segment struct {
	FlightNumber    string    `json:"flightNumber"`
	Airline         string    `json:"airline"`
	Aircraft        string    `json:"aircraft"`
	Origin          Airport   `json:"origin"`
	Destination     Airport   `json:"destination"`
	DepartureTime   time.Time `json:"departureTime"`
	ArrivalTime     time.Time `json:"arrivalTime"`
	DurationMinutes int       `json:"durationMinutes"`
	Price           float64   `json:"price"`
}

// Layover types reported by the API.
const (
	LayoverDomestic      = "domestic"
	LayoverInternational = "international"
)

// Layover is the wait between two consecutive segments.
type Layover struct {
	Airport         Airport `json:"airport"`
	DurationMinutes int     `json:"durationMinutes"`
	Type            string  `json:"type"` // LayoverDomestic or LayoverInternational
}

// Dataset describes the schedule the API has loaded.
type Dataset struct {
	Airports           int    `json:"airports"`
	Flights            int    `json:"flights"`
	FirstDepartureDate string `json:"firstDepartureDate"` // YYYY-MM-DD
	LastDepartureDate  string `json:"lastDepartureDate"`  // YYYY-MM-DD
	SkippedRecords     int    `json:"skippedRecords"`
}

// airportsResponse is the envelope around the airport list.
type airportsResponse struct {
	Count    int       `json:"count"`
	Airports []Airport `json:"airports"`
}
