// Package view defines the data the HTML templates render. It is the
// contract between the request handlers, which build these values from API
// responses, and the templates, which only display them.
//
// Every user-visible string arrives here already formatted (times, durations,
// prices), so templates contain no formatting logic and the formatting can be
// unit-tested in Go.
package view

// Page is everything the search page needs.
type Page struct {
	Title        string // document title
	AssetVersion string // appended to static asset URLs so new builds bypass caches

	Form     Form
	Airports []Airport // suggestions for the airport inputs; may be empty
	Dataset  *Dataset  // schedule coverage; nil when unknown
	Results  *Results  // nil until a search has completed successfully
	Notice   *Notice   // page-level message, e.g. search service unavailable
}

// Form is the search form's current values and any per-field errors. Values
// are echoed back exactly as the user typed them.
type Form struct {
	Origin      string
	Destination string
	Date        string // YYYY-MM-DD
	MinDate     string // optional date picker bounds, YYYY-MM-DD
	MaxDate     string
	Errors      FieldErrors
}

// FieldErrors holds one message per form field. An empty string means the
// field is valid.
type FieldErrors struct {
	Origin      string
	Destination string
	Date        string
}

// Any reports whether at least one field has an error.
func (e FieldErrors) Any() bool {
	return e.Origin != "" || e.Destination != "" || e.Date != ""
}

// Airport identifies an airport for display.
type Airport struct {
	Code    string // "JFK"
	Name    string // "John F. Kennedy International"
	City    string // "New York"
	Country string // "US"
}

// Dataset describes which dates the schedule covers, used for the date
// picker bounds and the empty-state hint.
type Dataset struct {
	FirstDate    string // display form, e.g. "Mar 15, 2024"
	LastDate     string
	FirstDateISO string // YYYY-MM-DD
	LastDateISO  string
}

// Results is the outcome of one search. An empty Itineraries slice means the
// search succeeded and found nothing.
type Results struct {
	Origin      Airport
	Destination Airport
	Date        string // display form, e.g. "Friday, March 15, 2024"
	DateISO     string // YYYY-MM-DD

	Count   int // total itineraries
	Direct  int // of which direct
	OneStop int // of which one stop
	TwoStop int // of which two stops

	Itineraries []Itinerary // fastest first
}

// Itinerary is one way of getting from origin to destination.
type Itinerary struct {
	Stops      int
	StopsLabel string   // "Direct", "1 stop", "2 stops"
	Via        []string // connecting airport codes, e.g. ["ORD"]; empty when direct

	Duration        string // "7h 30m"
	DurationMinutes int    // raw value, for client-side sorting
	Price           string // "$414.00"
	PriceCents      int64  // raw value, for client-side sorting

	Departure TimePoint // first departure
	Arrival   TimePoint // last arrival
	DayOffset string    // arrival date relative to departure date: "", "+1 day", "+2 days", "-1 day"

	Segments []Segment
}

// TimePoint is a moment at an airport, shown in that airport's local time.
type TimePoint struct {
	Time      string // "7:00 AM"
	Date      string // "Fri, Mar 15"
	ISO       string // RFC 3339 with offset, for the <time datetime> attribute
	UTCOffset string // "UTC-4"
	Airport   Airport
}

// Segment is a single flight within an itinerary.
type Segment struct {
	FlightNumber string
	Airline      string
	Aircraft     string

	Departure TimePoint
	Arrival   TimePoint
	DayOffset string // arrival date relative to this segment's departure date

	Duration string // "2h 30m"
	Price    string // "$189.00"

	Layover *Layover // the connection after this segment; nil on the last segment
}

// Layover is the wait between two segments.
type Layover struct {
	Airport  Airport
	Duration string // "45m", "1h 15m"
	Kind     string // "domestic" or "international"
	Label    string // "Domestic connection" or "International connection"
}

// Notice kinds.
const (
	NoticeError   = "error"
	NoticeWarning = "warning"
	NoticeInfo    = "info"
)

// Notice is a page-level message shown above the results.
type Notice struct {
	Kind    string // one of the Notice* constants
	Title   string
	Message string
}

// ErrorPage is the data for the standalone error page (404, 500).
type ErrorPage struct {
	Title        string // document title
	AssetVersion string
	Status       int    // HTTP status code
	Heading      string // "Page not found"
	Message      string
}
