package dataset

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/luisalarcon/skypath/backend/internal/flight"

	// Tests resolve real time zones; embedding the database keeps them
	// independent of what the host has installed.
	_ "time/tzdata"
)

// Two valid airports that every test document starts from.
const (
	jfkRecord = `{"code": "JFK", "name": "John F. Kennedy International", "city": "New York", "country": "US", "timezone": "America/New_York"}`
	laxRecord = `{"code": "LAX", "name": "Los Angeles International", "city": "Los Angeles", "country": "US", "timezone": "America/Los_Angeles"}`
)

// document assembles a schedule from raw JSON records.
func document(airports, flights []string) string {
	return `{"airports": [` + strings.Join(airports, ",") + `], "flights": [` + strings.Join(flights, ",") + `]}`
}

func mustParse(t *testing.T, doc string) *Dataset {
	t.Helper()
	d, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	return d
}

func airportCodes(airports []flight.Airport) []string {
	out := make([]string, len(airports))
	for i, a := range airports {
		out[i] = a.Code
	}
	return out
}

func TestParseAirport(t *testing.T) {
	d := mustParse(t, document([]string{
		`{"code": " lax ", "name": " Los Angeles International ", "city": "Los Angeles", "country": "us", "timezone": "America/Los_Angeles", "elevation": 38}`,
		jfkRecord,
		`{"code": "SFO", "name": "San Francisco International", "city": "San Francisco", "country": "US", "timezone": "America/Los_Angeles"}`,
	}, nil))

	if len(d.Skipped) != 0 {
		t.Fatalf("Skipped = %v, want none", d.Skipped)
	}
	if got, want := airportCodes(d.Airports), []string{"JFK", "LAX", "SFO"}; !slices.Equal(got, want) {
		t.Fatalf("airports = %v, want %v", got, want)
	}
	lax := d.Airports[1]
	if lax.Name != "Los Angeles International" || lax.City != "Los Angeles" || lax.Country != "US" {
		t.Errorf("LAX = %+v, want trimmed name and upper-case country", lax)
	}
	if lax.Location == nil || lax.Location.String() != "America/Los_Angeles" {
		t.Errorf("LAX location = %v, want America/Los_Angeles", lax.Location)
	}
	// Airports in one zone share a single loaded location.
	if d.Airports[2].Location != lax.Location {
		t.Errorf("LAX and SFO should share one *time.Location")
	}
}

func TestAirportSkipReasons(t *testing.T) {
	cases := []struct {
		name   string
		record string
		label  string
		reason string
	}{
		{"not an object", `42`, "airport #3", "record is a number, not a JSON object"},
		{"null record", `null`, "airport #3", "record is null, not a JSON object"},
		{"array record", `["ORD"]`, "airport #3", "record is an array, not a JSON object"},
		{"missing code", `{"name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", "missing code"},
		{"null code", `{"code": null, "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", "missing code"},
		{"blank code", `{"code": "  ", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", "empty code"},
		{"code not a string", `{"code": 123, "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", "code is a number, not a string"},
		{"code with a digit", `{"code": "O1D", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", `invalid code "O1D": want three letters A-Z`},
		{"code too short", `{"code": "OR", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", `invalid code "OR": want three letters A-Z`},
		{"code too long", `{"code": "ORDX", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", `invalid code "ORDX": want three letters A-Z`},
		{"code not ASCII", `{"code": "ÖRD", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport #3", `invalid code "ÖRD": want three letters A-Z`},
		{"duplicate code after normalising", `{"code": " jfk", "name": "Second JFK", "city": "New York", "country": "US", "timezone": "America/New_York"}`, "airport JFK", `duplicate airport code "JFK": the first airport with this code is kept`},
		{"duplicate is reported before other faults", `{"code": "JFK"}`, "airport JFK", `duplicate airport code "JFK": the first airport with this code is kept`},
		{"missing name", `{"code": "ORD", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport ORD", "missing name"},
		{"blank name", `{"code": "ORD", "name": " ", "city": "Chicago", "country": "US", "timezone": "America/Chicago"}`, "airport ORD", "empty name"},
		{"name before time zone", `{"code": "ORD", "city": "Chicago", "country": "US", "timezone": "Nowhere/Special"}`, "airport ORD", "missing name"},
		{"missing city", `{"code": "ORD", "name": "O'Hare", "country": "US", "timezone": "America/Chicago"}`, "airport ORD", "missing city"},
		{"city not a string", `{"code": "ORD", "name": "O'Hare", "city": ["Chicago"], "country": "US", "timezone": "America/Chicago"}`, "airport ORD", "city is an array, not a string"},
		{"missing country", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "timezone": "America/Chicago"}`, "airport ORD", "missing country"},
		{"three-letter country", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "USA", "timezone": "America/Chicago"}`, "airport ORD", `invalid country "USA": want two letters`},
		{"one-letter country", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "U", "timezone": "America/Chicago"}`, "airport ORD", `invalid country "U": want two letters`},
		{"country with a digit", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "U5", "timezone": "America/Chicago"}`, "airport ORD", `invalid country "U5": want two letters`},
		{"missing time zone", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "US"}`, "airport ORD", "missing time zone"},
		{"empty time zone", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": ""}`, "airport ORD", "empty time zone"},
		{"Local time zone", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "Local"}`, "airport ORD", `time zone "Local" is not allowed: name the airport's IANA zone`},
		{"unknown time zone", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "America/Gotham"}`, "airport ORD", `unknown time zone "America/Gotham"`},
		{"path as time zone", `{"code": "ORD", "name": "O'Hare", "city": "Chicago", "country": "US", "timezone": "../America/Chicago"}`, "airport ORD", `unknown time zone "../America/Chicago"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := mustParse(t, document([]string{jfkRecord, laxRecord, tc.record}, nil))
			want := []Skipped{{Record: tc.label, Reason: tc.reason}}
			if !slices.Equal(d.Skipped, want) {
				t.Errorf("Skipped = %q, want %q", d.Skipped, want)
			}
			if got := airportCodes(d.Airports); !slices.Equal(got, []string{"JFK", "LAX"}) {
				t.Errorf("airports = %v, want the two valid ones", got)
			}
		})
	}
}

func TestOnlyAnAcceptedAirportClaimsItsCode(t *testing.T) {
	d := mustParse(t, document([]string{
		`{"code": "JFK", "name": "Broken", "city": "New York", "country": "US", "timezone": "America/Gotham"}`,
		jfkRecord,
	}, nil))

	want := []Skipped{{Record: "airport JFK", Reason: `unknown time zone "America/Gotham"`}}
	if !slices.Equal(d.Skipped, want) {
		t.Errorf("Skipped = %q, want %q", d.Skipped, want)
	}
	if len(d.Airports) != 1 || d.Airports[0].Name != "John F. Kennedy International" {
		t.Errorf("airports = %+v, want the valid JFK", d.Airports)
	}
}

func TestAirportRecordIsolation(t *testing.T) {
	// A record of the wrong shape between two good ones must not affect them.
	d := mustParse(t, document([]string{jfkRecord, `{"code": {"nested": [1, 2]}, "name": 7}`, laxRecord}, nil))

	if got := airportCodes(d.Airports); !slices.Equal(got, []string{"JFK", "LAX"}) {
		t.Errorf("airports = %v, want JFK and LAX", got)
	}
	want := []Skipped{{Record: "airport #2", Reason: "code is an object, not a string"}}
	if !slices.Equal(d.Skipped, want) {
		t.Errorf("Skipped = %q, want %q", d.Skipped, want)
	}
}

func TestSkippedAirportsInFileOrder(t *testing.T) {
	d := mustParse(t, document([]string{
		`{"code": "ZZZ"}`,
		laxRecord,
		`{"code": "AAA"}`,
		`true`,
		jfkRecord,
	}, nil))

	want := []string{"airport ZZZ: missing name", "airport AAA: missing name", "airport #4: record is a boolean, not a JSON object"}
	var got []string
	for _, s := range d.Skipped {
		got = append(got, s.String())
	}
	if !slices.Equal(got, want) {
		t.Errorf("Skipped = %q, want %q", got, want)
	}
	if got := airportCodes(d.Airports); !slices.Equal(got, []string{"JFK", "LAX"}) {
		t.Errorf("airports = %v, want code order", got)
	}
}

func TestFileLevelFailures(t *testing.T) {
	valid := []string{jfkRecord, laxRecord}
	cases := []struct {
		name string
		doc  string
		want error
	}{
		{"empty input", ``, ErrInvalidJSON},
		{"truncated", `{"airports": [`, ErrInvalidJSON},
		{"trailing data", document(valid, nil) + ` {}`, ErrInvalidJSON},
		{"not JSON at all", `airports: []`, ErrInvalidJSON},
		{"top level is an array", `[]`, ErrMissingAirports},
		{"top level is null", `null`, ErrMissingAirports},
		{"no airports key", `{"flights": []}`, ErrMissingAirports},
		{"airports is null", `{"airports": null, "flights": []}`, ErrMissingAirports},
		{"airports is an object", `{"airports": {"JFK": {}}, "flights": []}`, ErrMissingAirports},
		{"key match is exact", `{"Airports": [` + jfkRecord + `], "flights": []}`, ErrMissingAirports},
		{"no flights key", `{"airports": [` + jfkRecord + `]}`, ErrMissingFlights},
		{"flights is a string", `{"airports": [` + jfkRecord + `], "flights": "none"}`, ErrMissingFlights},
		{"no airports", document(nil, nil), ErrNoValidAirports},
		{"no valid airport", document([]string{`{"code": "JFK"}`, `1`}, nil), ErrNoValidAirports},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(strings.NewReader(tc.doc))
			if !errors.Is(err, tc.want) {
				t.Fatalf("Parse error = %v, want %v", err, tc.want)
			}
			if d != nil {
				t.Errorf("Parse returned a dataset alongside the error")
			}
		})
	}
}

func TestNoValidAirportErrorNamesTheFirstReason(t *testing.T) {
	_, err := Parse(strings.NewReader(document([]string{`{"code": "JFK"}`, `1`}, nil)))
	if err == nil || !strings.Contains(err.Error(), "airport JFK: missing name") {
		t.Errorf("error = %v, want it to name the first skipped record", err)
	}
}

func TestUnreadableInput(t *testing.T) {
	cause := errors.New("disk on fire")
	_, err := Parse(iotest.ErrReader(cause))
	if !errors.Is(err, ErrUnreadable) || !errors.Is(err, cause) {
		t.Errorf("Parse error = %v, want ErrUnreadable wrapping the cause", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	_, err := Load(filepath.Join(dir, "missing.json"))
	if !errors.Is(err, ErrUnreadable) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load(missing) error = %v, want ErrUnreadable and fs.ErrNotExist", err)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); !errors.Is(err, ErrInvalidJSON) || !strings.Contains(err.Error(), bad) {
		t.Errorf("Load(bad) error = %v, want ErrInvalidJSON naming the file", err)
	}

	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(document([]string{jfkRecord, laxRecord}, nil)), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Load(good)
	if err != nil {
		t.Fatalf("Load(good) error = %v", err)
	}
	if len(d.Airports) != 2 {
		t.Errorf("Load(good) airports = %d, want 2", len(d.Airports))
	}
}
