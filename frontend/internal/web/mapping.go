package web

import (
	"time"

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/view"
)

// Tags that highlight an itinerary within its result set.
const (
	tagFastest  = "Fastest"
	tagCheapest = "Cheapest"
)

// isoDateLayout is the API's date format.
const isoDateLayout = "2006-01-02"

// airportView converts an API airport to its display form.
func airportView(a client.Airport) view.Airport {
	return view.Airport{Code: a.Code, Name: a.Name, City: a.City, Country: a.Country}
}

// timePoint describes a moment at an airport. The time is formatted in the
// offset it carries, which is the airport's local offset.
func timePoint(t time.Time, airport client.Airport) view.TimePoint {
	return view.TimePoint{
		Time:      formatTime(t),
		Date:      formatShortDate(t),
		ISO:       t.Format(time.RFC3339),
		UTCOffset: formatUTCOffset(t),
		Airport:   airportView(airport),
	}
}

// resultsView converts a completed search into the results the page shows.
//
// The API orders itineraries by total duration, so the first one is the
// fastest. Counts are taken from the itineraries themselves rather than the
// response's count field, so the numbers on the page always add up to the
// cards beneath them.
func resultsView(resp *client.SearchResponse) *view.Results {
	results := &view.Results{
		Origin:      airportView(resp.Origin),
		Destination: airportView(resp.Destination),
		Date:        resp.Date,
		DateISO:     resp.Date,
		Count:       len(resp.Itineraries),
		Itineraries: make([]view.Itinerary, 0, len(resp.Itineraries)),
	}
	// An unparseable date is shown as received rather than hiding the results.
	if date, err := time.Parse(isoDateLayout, resp.Date); err == nil {
		results.Date = formatLongDate(date)
	}

	for _, it := range resp.Itineraries {
		results.Itineraries = append(results.Itineraries, itineraryView(it, resp.Origin, resp.Destination))
		switch it.Stops {
		case 0:
			results.Direct++
		case 1:
			results.OneStop++
		default:
			results.TwoStop++
		}
	}
	addTags(results.Itineraries)
	return results
}

// itineraryView converts one itinerary. The search's own origin and
// destination stand in for the end airports of an itinerary that, against the
// contract, arrives without segments.
func itineraryView(it client.Itinerary, origin, destination client.Airport) view.Itinerary {
	if n := len(it.Segments); n > 0 {
		origin, destination = it.Segments[0].Origin, it.Segments[n-1].Destination
	}

	via := make([]string, 0, len(it.Layovers))
	for _, l := range it.Layovers {
		via = append(via, l.Airport.Code)
	}

	cents := priceCents(it.TotalPrice)
	return view.Itinerary{
		Stops:           it.Stops,
		StopsLabel:      formatStops(it.Stops),
		Via:             via,
		Duration:        formatDuration(it.TotalDurationMinutes),
		DurationMinutes: it.TotalDurationMinutes,
		Price:           formatPrice(cents),
		PriceCents:      cents,
		Departure:       timePoint(it.DepartureTime, origin),
		Arrival:         timePoint(it.ArrivalTime, destination),
		DayOffset:       formatDayOffset(it.DepartureTime, it.ArrivalTime),
		Segments:        segmentViews(it),
	}
}

// segmentViews converts the flights of an itinerary and attaches each
// connection to the flight it follows.
func segmentViews(it client.Itinerary) []view.Segment {
	segments := make([]view.Segment, 0, len(it.Segments))
	for i, s := range it.Segments {
		seg := view.Segment{
			FlightNumber: s.FlightNumber,
			Airline:      s.Airline,
			Aircraft:     s.Aircraft,
			Departure:    timePoint(s.DepartureTime, s.Origin),
			Arrival:      timePoint(s.ArrivalTime, s.Destination),
			DayOffset:    formatDayOffset(s.DepartureTime, s.ArrivalTime),
			Duration:     formatDuration(s.DurationMinutes),
			Price:        formatPrice(priceCents(s.Price)),
		}
		if i < len(it.Layovers) {
			seg.Layover = layoverView(it.Layovers[i])
		}
		segments = append(segments, seg)
	}
	return segments
}

func layoverView(l client.Layover) *view.Layover {
	kind, label := formatLayover(l.Type)
	return &view.Layover{
		Airport:  airportView(l.Airport),
		Duration: formatDuration(l.DurationMinutes),
		Kind:     kind,
		Label:    label,
	}
}

// addTags marks the fastest itinerary, which is first, and the cheapest. When
// several share the lowest price the earliest of them, which is also the
// fastest of them, is the one tagged. One itinerary can carry both tags.
func addTags(its []view.Itinerary) {
	if len(its) == 0 {
		return
	}
	its[0].Tags = append(its[0].Tags, tagFastest)

	cheapest := 0
	for i, it := range its {
		if it.PriceCents < its[cheapest].PriceCents {
			cheapest = i
		}
	}
	its[cheapest].Tags = append(its[cheapest].Tags, tagCheapest)
}
