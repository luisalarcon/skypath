package dataset

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

const nrtRecord = `{"code": "NRT", "name": "Narita International", "city": "Tokyo", "country": "JP", "timezone": "Asia/Tokyo"}`

var testAirports = []string{jfkRecord, laxRecord, nrtRecord}

// baseFlight is a valid JFK to LAX flight as raw JSON values by field.
var baseFlight = map[string]string{
	"flightNumber":  `"SP200"`,
	"airline":       `"SkyPath Airways"`,
	"origin":        `"JFK"`,
	"destination":   `"LAX"`,
	"departureTime": `"2024-03-15T08:30:00"`,
	"arrivalTime":   `"2024-03-15T11:45:00"`,
	"price":         `299.00`,
	"aircraft":      `"A320"`,
}

// flightJSON returns baseFlight with the given fields replaced or added and
// the dropped fields removed.
func flightJSON(set map[string]string, drop ...string) string {
	fields := maps.Clone(baseFlight)
	maps.Copy(fields, set)
	for _, key := range drop {
		delete(fields, key)
	}
	parts := make([]string, 0, len(fields))
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		parts = append(parts, `"`+key+`": `+fields[key])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func flightNumbers(flights []flight.Flight) []string {
	out := make([]string, len(flights))
	for i, f := range flights {
		out[i] = f.Number
	}
	return out
}

func TestParseFlight(t *testing.T) {
	d := mustParse(t, document(testAirports, []string{flightJSON(map[string]string{
		"flightNumber": `" SP200 "`,
		"origin":       `" jfk"`,
		"destination":  `"lax "`,
		"aircraft":     `" A320 "`,
		"gate":         `"B7"`,
	})}))

	if len(d.Skipped) != 0 || len(d.Flights) != 1 {
		t.Fatalf("flights = %d, skipped = %v; want one flight and no skips", len(d.Flights), d.Skipped)
	}
	f := d.Flights[0]
	if f.Number != "SP200" {
		t.Errorf("Number = %q, want trimmed SP200", f.Number)
	}
	if f.Origin != d.Airports[0] || f.Destination != d.Airports[1] {
		t.Errorf("endpoints = %s to %s, want the full JFK and LAX records", f.Origin.Code, f.Destination.Code)
	}
	if f.Airline != "SkyPath Airways" || f.Aircraft != " A320 " {
		t.Errorf("Airline, Aircraft = %q, %q, want them as given", f.Airline, f.Aircraft)
	}
	if f.Price != 29900 {
		t.Errorf("Price = %d, want 29900", f.Price)
	}

	// Each time is read in its own airport's zone and keeps that zone.
	if got := f.Departure.Location().String(); got != "America/New_York" {
		t.Errorf("departure zone = %s, want America/New_York", got)
	}
	if got := f.Arrival.Location().String(); got != "America/Los_Angeles" {
		t.Errorf("arrival zone = %s, want America/Los_Angeles", got)
	}
	if got, want := f.Departure.UTC(), time.Date(2024, time.March, 15, 12, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("departure = %s, want %s", got, want)
	}
	if got, want := f.Duration(), 375*time.Minute; got != want {
		t.Errorf("Duration = %s, want %s", got, want)
	}
}

func TestOptionalTextMayBeAbsent(t *testing.T) {
	d := mustParse(t, document(testAirports, []string{
		flightJSON(map[string]string{"airline": `null`}, "aircraft"),
	}))
	if len(d.Flights) != 1 || d.Flights[0].Airline != "" || d.Flights[0].Aircraft != "" {
		t.Errorf("flights = %+v, want one flight with empty airline and aircraft", d.Flights)
	}
}

func TestPriceForms(t *testing.T) {
	cases := []struct {
		raw  string
		want flight.Money
	}{
		{`299.00`, 29900},
		{`299.5`, 29950},
		{`109`, 10900},
		{`0`, 0},
		{`0.10`, 10},
		{`"99"`, 9900},
		{`"95.00"`, 9500},
		{`"289.05"`, 28905},
		{`" 99 "`, 9900},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			d := mustParse(t, document(testAirports, []string{flightJSON(map[string]string{"price": tc.raw})}))
			if len(d.Flights) != 1 {
				t.Fatalf("flight skipped: %v", d.Skipped)
			}
			if got := d.Flights[0].Price; got != tc.want {
				t.Errorf("price %s = %d cents, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestFlightSkipReasons(t *testing.T) {
	const priceRule = ": want a non-negative amount with at most two decimal places"
	const layoutRule = ": want YYYY-MM-DDTHH:MM:SS"
	cases := []struct {
		name   string
		record string
		label  string
		reason string
	}{
		{"not an object", `"SP300"`, "flight #2", "record is a string, not a JSON object"},
		{"null record", `null`, "flight #2", "record is null, not a JSON object"},
		{"missing number", flightJSON(nil, "flightNumber"), "flight #2", "missing flight number"},
		{"blank number", flightJSON(map[string]string{"flightNumber": `"   "`}), "flight #2", "empty flight number"},
		{"number not a string", flightJSON(map[string]string{"flightNumber": `300`}), "flight #2", "flight number is a number, not a string"},
		{"duplicate number", flightJSON(map[string]string{"flightNumber": `"SP100"`}), "flight SP100", `duplicate flight number "SP100": the first flight with this number is kept`},
		{"duplicate after trimming", flightJSON(map[string]string{"flightNumber": `" SP100"`}), "flight SP100", `duplicate flight number "SP100": the first flight with this number is kept`},
		{"duplicate is reported before other faults", `{"flightNumber": "SP100"}`, "flight SP100", `duplicate flight number "SP100": the first flight with this number is kept`},
		{"missing origin", flightJSON(nil, "origin"), "flight SP200", "missing origin airport"},
		{"origin not a string", flightJSON(map[string]string{"origin": `{"code": "JFK"}`}), "flight SP200", "origin airport is an object, not a string"},
		{"unknown origin", flightJSON(map[string]string{"origin": `"JKF"`}), "flight SP200", `unknown origin airport "JKF"`},
		{"origin checked before destination", flightJSON(map[string]string{"origin": `"JKF"`, "destination": `"XXX"`}), "flight SP200", `unknown origin airport "JKF"`},
		{"missing destination", flightJSON(nil, "destination"), "flight SP200", "missing destination airport"},
		{"unknown destination", flightJSON(map[string]string{"destination": `"LAXX"`}), "flight SP200", `unknown destination airport "LAXX"`},
		{"same endpoints", flightJSON(map[string]string{"destination": `"jfk"`}), "flight SP200", `origin and destination are the same airport "JFK"`},
		{"missing departure", flightJSON(nil, "departureTime"), "flight SP200", "missing departure time"},
		{"departure not a string", flightJSON(map[string]string{"departureTime": `1710505800`}), "flight SP200", "departure time is a number, not a string"},
		{"space separator", flightJSON(map[string]string{"departureTime": `"2024-03-15 08:30:00"`}), "flight SP200", `invalid departure time "2024-03-15 08:30:00"` + layoutRule},
		{"one-digit hour", flightJSON(map[string]string{"departureTime": `"2024-03-15T8:30:00"`}), "flight SP200", `invalid departure time "2024-03-15T8:30:00"` + layoutRule},
		{"fractional seconds", flightJSON(map[string]string{"departureTime": `"2024-03-15T08:30:00.000"`}), "flight SP200", `invalid departure time "2024-03-15T08:30:00.000"` + layoutRule},
		{"UTC designator", flightJSON(map[string]string{"departureTime": `"2024-03-15T08:30:00Z"`}), "flight SP200", `invalid departure time "2024-03-15T08:30:00Z"` + layoutRule},
		{"offset", flightJSON(map[string]string{"departureTime": `"2024-03-15T08:30:00-04:00"`}), "flight SP200", `invalid departure time "2024-03-15T08:30:00-04:00"` + layoutRule},
		{"no seconds", flightJSON(map[string]string{"departureTime": `"2024-03-15T08:30"`}), "flight SP200", `invalid departure time "2024-03-15T08:30"` + layoutRule},
		{"surrounding space", flightJSON(map[string]string{"departureTime": `" 2024-03-15T08:30:00"`}), "flight SP200", `invalid departure time " 2024-03-15T08:30:00"` + layoutRule},
		{"no such date", flightJSON(map[string]string{"departureTime": `"2024-02-30T08:30:00"`}), "flight SP200", `invalid departure time "2024-02-30T08:30:00": not a real date and time`},
		{"hour 24", flightJSON(map[string]string{"departureTime": `"2024-03-15T24:00:00"`}), "flight SP200", `invalid departure time "2024-03-15T24:00:00": not a real date and time`},
		{"inside a daylight saving gap", flightJSON(map[string]string{"departureTime": `"2024-03-10T02:30:00"`, "arrivalTime": `"2024-03-10T09:00:00"`}), "flight SP200", `invalid departure time "2024-03-10T02:30:00": that local time does not exist in America/New_York`},
		{"departure checked before arrival", flightJSON(map[string]string{"departureTime": `"soon"`, "arrivalTime": `"later"`}), "flight SP200", `invalid departure time "soon"` + layoutRule},
		{"missing arrival", flightJSON(nil, "arrivalTime"), "flight SP200", "missing arrival time"},
		{"invalid arrival", flightJSON(map[string]string{"arrivalTime": `"2024-03-15T11:45"`}), "flight SP200", `invalid arrival time "2024-03-15T11:45"` + layoutRule},
		{"arrival at the departure instant", flightJSON(map[string]string{"departureTime": `"2024-03-15T08:00:00"`, "arrivalTime": `"2024-03-15T05:00:00"`}), "flight SP200", "arrival (2024-03-15T05:00:00 America/Los_Angeles) is not after departure (2024-03-15T08:00:00 America/New_York)"},
		{"arrival before departure", flightJSON(map[string]string{"arrivalTime": `"2024-03-15T05:29:59"`}), "flight SP200", "arrival (2024-03-15T05:29:59 America/Los_Angeles) is not after departure (2024-03-15T08:30:00 America/New_York)"},
		{"order checked before price", flightJSON(map[string]string{"arrivalTime": `"2024-03-15T05:00:00"`, "price": `-1`}), "flight SP200", "arrival (2024-03-15T05:00:00 America/Los_Angeles) is not after departure (2024-03-15T08:30:00 America/New_York)"},
		{"missing price", flightJSON(nil, "price"), "flight SP200", "missing price"},
		{"null price", flightJSON(map[string]string{"price": `null`}), "flight SP200", "missing price"},
		{"negative price", flightJSON(map[string]string{"price": `-1`}), "flight SP200", "invalid price -1" + priceRule},
		{"negative price as text", flightJSON(map[string]string{"price": `"-1.00"`}), "flight SP200", `invalid price "-1.00"` + priceRule},
		{"three decimals", flightJSON(map[string]string{"price": `299.001`}), "flight SP200", "invalid price 299.001" + priceRule},
		{"three decimals as text", flightJSON(map[string]string{"price": `"299.001"`}), "flight SP200", `invalid price "299.001"` + priceRule},
		{"exponent", flightJSON(map[string]string{"price": `2.99e2`}), "flight SP200", "invalid price 2.99e2" + priceRule},
		{"currency symbol", flightJSON(map[string]string{"price": `"$299"`}), "flight SP200", `invalid price "$299"` + priceRule},
		{"empty text", flightJSON(map[string]string{"price": `""`}), "flight SP200", `invalid price ""` + priceRule},
		{"boolean price", flightJSON(map[string]string{"price": `true`}), "flight SP200", "invalid price true" + priceRule},
		{"object price", flightJSON(map[string]string{"price": `{ "amount": 299 }`}), "flight SP200", `invalid price {"amount":299}` + priceRule},
		{"long price is shortened", flightJSON(map[string]string{"price": `"` + strings.Repeat("9", 60) + `"`}), "flight SP200", `invalid price "` + strings.Repeat("9", 36) + `...` + priceRule},
		{"airline not a string", flightJSON(map[string]string{"airline": `7`}), "flight SP200", "airline is a number, not a string"},
		{"aircraft not a string", flightJSON(map[string]string{"aircraft": `["A320"]`}), "flight SP200", "aircraft is an array, not a string"},
	}
	first := flightJSON(map[string]string{"flightNumber": `"SP100"`})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := mustParse(t, document(testAirports, []string{first, tc.record}))
			want := []Skipped{{Record: tc.label, Reason: tc.reason}}
			if !slices.Equal(d.Skipped, want) {
				t.Errorf("Skipped = %q\nwant      %q", d.Skipped, want)
			}
			if got := flightNumbers(d.Flights); !slices.Equal(got, []string{"SP100"}) {
				t.Errorf("flights = %v, want only SP100", got)
			}
		})
	}
}

func TestOnlyAnAcceptedFlightClaimsItsNumber(t *testing.T) {
	d := mustParse(t, document(testAirports, []string{
		flightJSON(map[string]string{"origin": `"JKF"`}),
		flightJSON(map[string]string{"price": `150`}),
	}))
	want := []Skipped{{Record: "flight SP200", Reason: `unknown origin airport "JKF"`}}
	if !slices.Equal(d.Skipped, want) {
		t.Errorf("Skipped = %q, want %q", d.Skipped, want)
	}
	if len(d.Flights) != 1 || d.Flights[0].Price != 15000 {
		t.Errorf("flights = %+v, want the valid SP200", d.Flights)
	}
}

func TestFlightRecordIsolation(t *testing.T) {
	// A record that would fail to decode as a whole sits between two good
	// ones; only it is skipped.
	d := mustParse(t, document(testAirports, []string{
		flightJSON(map[string]string{"flightNumber": `"SP100"`}),
		flightJSON(map[string]string{"flightNumber": `"SP150"`, "price": `[1, 2]`, "departureTime": `{}`}),
		flightJSON(map[string]string{"flightNumber": `"SP300"`}),
	}))
	if got := flightNumbers(d.Flights); !slices.Equal(got, []string{"SP100", "SP300"}) {
		t.Errorf("flights = %v, want SP100 and SP300", got)
	}
	want := []Skipped{{Record: "flight SP150", Reason: "departure time is an object, not a string"}}
	if !slices.Equal(d.Skipped, want) {
		t.Errorf("Skipped = %q, want %q", d.Skipped, want)
	}
}

func TestOutputOrder(t *testing.T) {
	d := mustParse(t, document(
		[]string{nrtRecord, `{"code": "BAD"}`, laxRecord, jfkRecord},
		[]string{
			flightJSON(map[string]string{"flightNumber": `"SP900"`, "departureTime": `"2024-03-15T20:00:00"`, "arrivalTime": `"2024-03-15T23:15:00"`}),
			`[]`,
			flightJSON(map[string]string{"flightNumber": `"SP100"`}),
			flightJSON(map[string]string{"flightNumber": `"SP500"`, "origin": `"MIA"`}),
			flightJSON(map[string]string{"flightNumber": `"SP300"`, "departureTime": `"2024-03-15T06:00:00"`, "arrivalTime": `"2024-03-15T09:15:00"`}),
		},
	))

	if got, want := airportCodes(d.Airports), []string{"JFK", "LAX", "NRT"}; !slices.Equal(got, want) {
		t.Errorf("airports = %v, want code order %v", got, want)
	}
	// Flights keep file order; the store, not the loader, orders by time.
	if got, want := flightNumbers(d.Flights), []string{"SP900", "SP100", "SP300"}; !slices.Equal(got, want) {
		t.Errorf("flights = %v, want file order %v", got, want)
	}
	var got []string
	for _, s := range d.Skipped {
		got = append(got, s.String())
	}
	want := []string{
		"airport BAD: missing name",
		"flight #2: record is an array, not a JSON object",
		`flight SP500: unknown origin airport "MIA"`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("Skipped = %q, want %q", got, want)
	}
}

func TestNoValidFlight(t *testing.T) {
	cases := []struct {
		name    string
		flights []string
		detail  string
	}{
		{"empty array", nil, "the array is empty"},
		{"every record skipped", []string{flightJSON(map[string]string{"origin": `"JKF"`}), `7`}, `all 2 records were skipped, the first as flight SP200: unknown origin airport "JKF"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The bad airport must not be mistaken for the first bad flight.
			_, err := Parse(strings.NewReader(document(append([]string{`{"code": "BAD"}`}, testAirports...), tc.flights)))
			if !errors.Is(err, ErrNoValidFlights) {
				t.Fatalf("Parse error = %v, want ErrNoValidFlights", err)
			}
			if !strings.Contains(err.Error(), tc.detail) {
				t.Errorf("error = %q, want it to contain %q", err, tc.detail)
			}
		})
	}
}
