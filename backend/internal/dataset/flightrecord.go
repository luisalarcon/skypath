package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// localLayout is the only accepted form of a schedule time: a local wall
// clock reading with no offset, interpreted in its airport's zone.
const localLayout = "2006-01-02T15:04:05"

// parseFlight validates one flight record, applying the checks in the order
// docs/data.md lists them so the first failure is the reason given.
//
// airports holds the accepted airports by code and accepted the flight
// numbers loaded so far. On failure the returned flight carries its number
// whenever one was given, so the record can be labelled by it.
func parseFlight(raw json.RawMessage, airports map[string]flight.Airport, accepted map[string]bool) (flight.Flight, error) {
	r, err := decodeRecord(raw)
	if err != nil {
		return flight.Flight{}, err
	}

	number, err := r.requiredText("flightNumber", "flight number")
	if err != nil {
		return flight.Flight{}, err
	}
	f := flight.Flight{Number: number}

	// Only accepted flights claim a number, so a valid record still loads
	// when an earlier one with the same number was skipped.
	if accepted[number] {
		return f, fmt.Errorf("duplicate flight number %q: the first flight with this number is kept", number)
	}

	if f.Origin, err = endpoint(r, "origin", airports); err != nil {
		return f, err
	}
	if f.Destination, err = endpoint(r, "destination", airports); err != nil {
		return f, err
	}
	if f.Origin.Code == f.Destination.Code {
		return f, fmt.Errorf("origin and destination are the same airport %q", f.Origin.Code)
	}

	if f.Departure, err = localTime(r, "departureTime", "departure time", f.Origin.Location); err != nil {
		return f, err
	}
	if f.Arrival, err = localTime(r, "arrivalTime", "arrival time", f.Destination.Location); err != nil {
		return f, err
	}
	if !f.Arrival.After(f.Departure) {
		return f, fmt.Errorf("arrival (%s) is not after departure (%s)", describeInstant(f.Arrival), describeInstant(f.Departure))
	}

	if !r.has("price") {
		return f, fmt.Errorf("missing price")
	}
	// Money reads the number or string from its text form straight into
	// cents, so the value never passes through floating point.
	if err := f.Price.UnmarshalJSON(r["price"]); err != nil {
		return f, fmt.Errorf("invalid price %s: want a non-negative amount with at most two decimal places", display(r["price"]))
	}

	if f.Airline, err = r.optionalText("airline", "airline"); err != nil {
		return f, err
	}
	if f.Aircraft, err = r.optionalText("aircraft", "aircraft"); err != nil {
		return f, err
	}
	return f, nil
}

// endpoint resolves the origin or destination field to a loaded airport.
func endpoint(r record, key string, airports map[string]flight.Airport) (flight.Airport, error) {
	given, err := r.requiredText(key, key+" airport")
	if err != nil {
		return flight.Airport{}, err
	}
	a, ok := airports[flight.NormalizeCode(given)]
	if !ok {
		// No attempt is made to correct a near miss such as "JKF": a wrong
		// guess would invent a flight, while a skipped one is visible.
		return flight.Airport{}, fmt.Errorf("unknown %s airport %q", key, given)
	}
	return a, nil
}

// localTime reads a schedule time field in the given airport zone.
func localTime(r record, key, name string, loc *time.Location) (time.Time, error) {
	if !r.has(key) {
		return time.Time{}, fmt.Errorf("missing %s", name)
	}
	s, err := r.optionalText(key, name)
	if err != nil {
		return time.Time{}, err
	}
	// time.Parse alone is too lenient for "exactly this layout": it accepts a
	// one-digit hour and a fractional second the layout does not mention. The
	// shape is therefore checked first and Parse left to check the ranges.
	if !hasLocalShape(s) {
		return time.Time{}, fmt.Errorf("invalid %s %q: want YYYY-MM-DDTHH:MM:SS", name, s)
	}
	t, err := time.ParseInLocation(localLayout, s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s %q: not a real date and time", name, s)
	}
	// A wall clock reading inside a daylight saving gap never happens in that
	// zone. ParseInLocation silently moves it to a nearby instant, which
	// would be a guess, so a reading that does not survive the round trip is
	// rejected.
	if t.Format(localLayout) != s {
		return time.Time{}, fmt.Errorf("invalid %s %q: that local time does not exist in %s", name, s, loc)
	}
	return t, nil
}

// hasLocalShape reports whether s is laid out exactly as YYYY-MM-DDTHH:MM:SS
// with ASCII digits in every numeric position.
func hasLocalShape(s string) bool {
	const shape = "dddd-dd-ddTdd:dd:dd"
	if len(s) != len(shape) {
		return false
	}
	for i := 0; i < len(shape); i++ {
		if shape[i] == 'd' {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		} else if s[i] != shape[i] {
			return false
		}
	}
	return true
}

// describeInstant formats an instant as its airport's local time and zone,
// which is how the schedule file states it.
func describeInstant(t time.Time) string {
	return t.Format(localLayout) + " " + t.Location().String()
}

// display renders a raw JSON value for a reason, compacted and shortened so
// a huge or multi-line value cannot swamp the load summary.
func display(raw json.RawMessage) string {
	const limit = 40
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return kindOf(raw)
	}
	s := buf.String()
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-3]) + "..."
}
