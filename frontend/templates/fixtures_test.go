package templates

import "github.com/luisalarcon/skypath/frontend/internal/view"

// Fixtures: one view model per page state, built from the payloads in
// api/examples so the templates are exercised with realistic values. The
// handlers produce exactly these shapes; every string is already formatted.

const testAssetVersion = "test-1"

var testAirports = []view.Airport{
	{Code: "JFK", Name: "John F. Kennedy International", City: "New York", Country: "US"},
	{Code: "LGA", Name: "LaGuardia Airport", City: "New York", Country: "US"},
	{Code: "EWR", Name: "Newark Liberty International", City: "New York", Country: "US"},
	{Code: "LAX", Name: "Los Angeles International", City: "Los Angeles", Country: "US"},
	{Code: "SFO", Name: "San Francisco International", City: "San Francisco", Country: "US"},
	{Code: "ORD", Name: "O'Hare International", City: "Chicago", Country: "US"},
	{Code: "DFW", Name: "Dallas/Fort Worth International", City: "Dallas", Country: "US"},
	{Code: "DEN", Name: "Denver International", City: "Denver", Country: "US"},
	{Code: "SEA", Name: "Seattle-Tacoma International", City: "Seattle", Country: "US"},
	{Code: "BOS", Name: "Boston Logan International", City: "Boston", Country: "US"},
	{Code: "MIA", Name: "Miami International", City: "Miami", Country: "US"},
	{Code: "ATL", Name: "Hartsfield-Jackson Atlanta International", City: "Atlanta", Country: "US"},
	{Code: "PHX", Name: "Phoenix Sky Harbor International", City: "Phoenix", Country: "US"},
	{Code: "LHR", Name: "London Heathrow", City: "London", Country: "GB"},
	{Code: "CDG", Name: "Charles de Gaulle", City: "Paris", Country: "FR"},
	{Code: "FRA", Name: "Frankfurt Airport", City: "Frankfurt", Country: "DE"},
	{Code: "AMS", Name: "Amsterdam Schiphol", City: "Amsterdam", Country: "NL"},
	{Code: "NRT", Name: "Narita International", City: "Tokyo", Country: "JP"},
	{Code: "HND", Name: "Tokyo Haneda", City: "Tokyo", Country: "JP"},
	{Code: "SYD", Name: "Sydney Kingsford Smith", City: "Sydney", Country: "AU"},
	{Code: "SIN", Name: "Singapore Changi", City: "Singapore", Country: "SG"},
	{Code: "HKG", Name: "Hong Kong International", City: "Hong Kong", Country: "HK"},
	{Code: "DXB", Name: "Dubai International", City: "Dubai", Country: "AE"},
	{Code: "YYZ", Name: "Toronto Pearson International", City: "Toronto", Country: "CA"},
	{Code: "MEX", Name: "Mexico City International", City: "Mexico City", Country: "MX"},
}

func testAirport(code string) view.Airport {
	for _, a := range testAirports {
		if a.Code == code {
			return a
		}
	}
	panic("fixture airport not found: " + code)
}

func testDataset() *view.Dataset {
	return &view.Dataset{
		FirstDate:    "Mar 15, 2024",
		LastDate:     "Mar 16, 2024",
		FirstDateISO: "2024-03-15",
		LastDateISO:  "2024-03-16",
	}
}

func point(clock, date, iso, offset, code string) view.TimePoint {
	return view.TimePoint{Time: clock, Date: date, ISO: iso, UTCOffset: offset, Airport: testAirport(code)}
}

// basePage is the page every state starts from: suggestions, coverage and a
// form defaulting to the first covered date.
func basePage() view.Page {
	return view.Page{
		Title:        "SkyPath · Flight search",
		AssetVersion: testAssetVersion,
		Form: view.Form{
			Date:    "2024-03-15",
			MinDate: "2024-03-15",
			MaxDate: "2024-03-16",
		},
		Airports: testAirports,
		Dataset:  testDataset(),
	}
}

// initialPage is the page before any search.
func initialPage() view.Page {
	return basePage()
}

// resultsPage is JFK to LAX on Mar 15 (api/examples/search-success.json): a
// direct flight that is both fastest and cheapest, and a one-stop via ORD.
func resultsPage() view.Page {
	p := basePage()
	p.Title = "JFK to LAX · Mar 15, 2024 · SkyPath"
	p.Form.Origin, p.Form.Destination = "JFK", "LAX"
	p.Form.OriginName = "New York · John F. Kennedy International"
	p.Form.DestinationName = "Los Angeles · Los Angeles International"
	p.Results = &view.Results{
		Origin:      testAirport("JFK"),
		Destination: testAirport("LAX"),
		Date:        "Friday, March 15, 2024",
		DateISO:     "2024-03-15",
		Count:       2,
		Direct:      1,
		OneStop:     1,
		Summary:     []string{"2 itineraries", "1 direct", "1 one stop"},
		Itineraries: []view.Itinerary{
			{
				Stops:           0,
				StopsLabel:      "Direct",
				Tags:            []string{"Fastest", "Cheapest"},
				Duration:        "6h 15m",
				DurationMinutes: 375,
				Price:           "$299.00",
				PriceCents:      29900,
				Departure:       point("8:30 AM", "Fri, Mar 15", "2024-03-15T08:30:00-04:00", "UTC-4", "JFK"),
				Arrival:         point("11:45 AM", "Fri, Mar 15", "2024-03-15T11:45:00-07:00", "UTC-7", "LAX"),
				Segments: []view.Segment{
					{
						FlightNumber: "SP101", Airline: "SkyPath Airways", Aircraft: "A320",
						Departure: point("8:30 AM", "Fri, Mar 15", "2024-03-15T08:30:00-04:00", "UTC-4", "JFK"),
						Arrival:   point("11:45 AM", "Fri, Mar 15", "2024-03-15T11:45:00-07:00", "UTC-7", "LAX"),
						Duration:  "6h 15m", Price: "$299.00",
					},
				},
			},
			{
				Stops:           1,
				StopsLabel:      "1 stop",
				Via:             []string{"ORD"},
				Duration:        "7h 30m",
				DurationMinutes: 450,
				Price:           "$414.00",
				PriceCents:      41400,
				Departure:       point("7:00 AM", "Fri, Mar 15", "2024-03-15T07:00:00-04:00", "UTC-4", "JFK"),
				Arrival:         point("11:30 AM", "Fri, Mar 15", "2024-03-15T11:30:00-07:00", "UTC-7", "LAX"),
				Segments: []view.Segment{
					{
						FlightNumber: "SP110", Airline: "SkyPath Airways", Aircraft: "A319",
						Departure: point("7:00 AM", "Fri, Mar 15", "2024-03-15T07:00:00-04:00", "UTC-4", "JFK"),
						Arrival:   point("8:30 AM", "Fri, Mar 15", "2024-03-15T08:30:00-05:00", "UTC-5", "ORD"),
						Duration:  "2h 30m", Price: "$189.00",
						Layover: &view.Layover{
							Airport: testAirport("ORD"), Duration: "45m",
							Kind: "domestic", Label: "Domestic connection",
						},
					},
					{
						FlightNumber: "SP120", Airline: "SkyPath Airways", Aircraft: "A320",
						Departure: point("9:15 AM", "Fri, Mar 15", "2024-03-15T09:15:00-05:00", "UTC-5", "ORD"),
						Arrival:   point("11:30 AM", "Fri, Mar 15", "2024-03-15T11:30:00-07:00", "UTC-7", "LAX"),
						Duration:  "4h 15m", Price: "$225.00",
					},
				},
			},
		},
	}
	return p
}

// twoStopPage is SFO to NRT (api/examples/search-two-stops.json): two
// connections, one of them international, arriving two calendar days later.
func twoStopPage() view.Page {
	p := basePage()
	p.Title = "SFO to NRT · Mar 15, 2024 · SkyPath"
	p.Form.Origin, p.Form.Destination = "SFO", "NRT"
	p.Form.OriginName = "San Francisco · San Francisco International"
	p.Form.DestinationName = "Tokyo · Narita International"
	p.Results = &view.Results{
		Origin:      testAirport("SFO"),
		Destination: testAirport("NRT"),
		Date:        "Friday, March 15, 2024",
		DateISO:     "2024-03-15",
		Count:       1,
		TwoStop:     1,
		Summary:     []string{"1 itinerary", "1 two stops"},
		Itineraries: []view.Itinerary{
			{
				Stops:           2,
				StopsLabel:      "2 stops",
				Via:             []string{"DEN", "LAX"},
				Tags:            []string{"Fastest", "Cheapest"},
				Duration:        "21h",
				DurationMinutes: 1260,
				Price:           "$1,137.00",
				PriceCents:      113700,
				Departure:       point("2:00 PM", "Fri, Mar 15", "2024-03-15T14:00:00-07:00", "UTC-7", "SFO"),
				Arrival:         point("3:00 AM", "Sun, Mar 17", "2024-03-17T03:00:00+09:00", "UTC+9", "NRT"),
				DayOffset:       "+2 days",
				Segments: []view.Segment{
					{
						FlightNumber: "SP981", Airline: "SkyPath Airways", Aircraft: "B737",
						Departure: point("2:00 PM", "Fri, Mar 15", "2024-03-15T14:00:00-07:00", "UTC-7", "SFO"),
						Arrival:   point("5:30 PM", "Fri, Mar 15", "2024-03-15T17:30:00-06:00", "UTC-6", "DEN"),
						Duration:  "2h 30m", Price: "$199.00",
						Layover: &view.Layover{
							Airport: testAirport("DEN"), Duration: "1h 30m",
							Kind: "domestic", Label: "Domestic connection",
						},
					},
					{
						FlightNumber: "SP212", Airline: "SkyPath Airways", Aircraft: "A320",
						Departure: point("7:00 PM", "Fri, Mar 15", "2024-03-15T19:00:00-06:00", "UTC-6", "DEN"),
						Arrival:   point("8:15 PM", "Fri, Mar 15", "2024-03-15T20:15:00-07:00", "UTC-7", "LAX"),
						Duration:  "2h 15m", Price: "$139.00",
						Layover: &view.Layover{
							Airport: testAirport("LAX"), Duration: "2h 45m",
							Kind: "international", Label: "International connection",
						},
					},
					{
						FlightNumber: "SP501", Airline: "SkyPath Airways", Aircraft: "B777",
						Departure: point("11:00 PM", "Fri, Mar 15", "2024-03-15T23:00:00-07:00", "UTC-7", "LAX"),
						Arrival:   point("3:00 AM", "Sun, Mar 17", "2024-03-17T03:00:00+09:00", "UTC+9", "NRT"),
						DayOffset: "+2 days",
						Duration:  "12h", Price: "$799.00",
					},
				},
			},
		},
	}
	return p
}

// emptyPage is a valid search with no itineraries
// (api/examples/search-empty.json), on a date outside the schedule.
func emptyPage() view.Page {
	p := basePage()
	p.Title = "JFK to LAX · Mar 20, 2024 · SkyPath"
	p.Form.Origin, p.Form.Destination, p.Form.Date = "JFK", "LAX", "2024-03-20"
	p.Form.OriginName = "New York · John F. Kennedy International"
	p.Form.DestinationName = "Los Angeles · Los Angeles International"
	p.Results = &view.Results{
		Origin:      testAirport("JFK"),
		Destination: testAirport("LAX"),
		Date:        "Wednesday, March 20, 2024",
		DateISO:     "2024-03-20",
		Summary:     []string{"0 itineraries"},
		Retry: &view.Link{
			Label: "Search Mar 15, 2024 instead",
			URL:   "/?date=2024-03-15&destination=LAX&origin=JFK",
		},
	}
	return p
}

// invalidPage carries a field error on every input, as the API reports them.
func invalidPage() view.Page {
	p := basePage()
	p.Form.Origin, p.Form.Destination, p.Form.Date = "XXX", "", "2024-13-45"
	p.Form.Errors = view.FieldErrors{
		Origin:      "Unknown airport code XXX.",
		Destination: "Destination is required.",
		Date:        "Date must be a valid date in YYYY-MM-DD format.",
	}
	return p
}

// unavailablePage is a search the service could not answer. The suggestions
// and coverage come from the same service, so they are missing too.
func unavailablePage() view.Page {
	p := basePage()
	p.Title = "JFK to LAX · SkyPath"
	p.Form.Origin, p.Form.Destination = "JFK", "LAX"
	p.Airports = nil
	p.Dataset = nil
	p.Form.MinDate, p.Form.MaxDate = "", ""
	p.Notice = &view.Notice{
		Kind:    view.NoticeError,
		Title:   "Search is temporarily unavailable",
		Message: "We couldn’t reach the flight search service. Your search is still filled in, so try again in a moment.",
	}
	return p
}

func notFoundPage() view.ErrorPage {
	return view.ErrorPage{
		Title:        "Page not found · SkyPath",
		AssetVersion: testAssetVersion,
		Status:       404,
		Heading:      "Page not found",
		Message:      "There is nothing at this address. Check the link, or start a new search.",
	}
}

func serverErrorPage() view.ErrorPage {
	return view.ErrorPage{
		Title:        "Something went wrong · SkyPath",
		AssetVersion: testAssetVersion,
		Status:       500,
		Heading:      "Something went wrong",
		Message:      "The page could not be shown. Please try again in a moment.",
	}
}

// hostile is placed in every user-influenced field to prove it is escaped
// in text, attribute and URL contexts alike.
const hostile = `"><script>alert(1)</script><img src=x onerror=alert(2)>`

func hostilePage() view.Page {
	p := resultsPage()
	p.Title = hostile
	p.Form.Origin, p.Form.Destination, p.Form.Date = hostile, hostile, hostile
	p.Form.MinDate, p.Form.MaxDate = hostile, hostile
	p.Form.OriginName, p.Form.DestinationName = hostile, hostile
	p.Form.Errors = view.FieldErrors{Origin: hostile, Destination: hostile, Date: hostile}
	p.Notice = &view.Notice{Kind: hostile, Title: hostile, Message: hostile}
	p.Results.Origin.City, p.Results.Origin.Code = hostile, hostile
	p.Results.Date, p.Results.DateISO = hostile, hostile
	return p
}

// hostileEmptyPage covers the empty state, whose retry link is built from
// the result's airport codes.
func hostileEmptyPage() view.Page {
	p := emptyPage()
	p.Results.Origin.Code, p.Results.Destination.Code = hostile, hostile
	p.Results.Origin.City = hostile
	return p
}

// pageFixture names a search page state.
type pageFixture struct {
	name string
	page view.Page
}

func pageFixtures() []pageFixture {
	return []pageFixture{
		{"initial", initialPage()},
		{"results", resultsPage()},
		{"two-stops", twoStopPage()},
		{"empty", emptyPage()},
		{"invalid", invalidPage()},
		{"unavailable", unavailablePage()},
	}
}

// errorFixture names an error page.
type errorFixture struct {
	name string
	page view.ErrorPage
}

func errorFixtures() []errorFixture {
	return []errorFixture{
		{"not-found", notFoundPage()},
		{"server-error", serverErrorPage()},
	}
}
