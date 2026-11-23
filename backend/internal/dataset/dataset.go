package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// Dataset is the validated content of a schedule file.
type Dataset struct {
	Airports []flight.Airport // ordered by code
	Flights  []flight.Flight  // every valid flight, in file order
	Skipped  []Skipped        // rejected airports, then rejected flights, each in file order
}

// Skipped describes one record that was rejected while loading.
type Skipped struct {
	// Record identifies the record: "airport JFK" or "flight SP995" when
	// that field is usable, otherwise its 1-based position in its array,
	// such as "flight #12".
	Record string
	// Reason says in plain language why the record was rejected, for
	// example `unknown origin airport "JKF"`.
	Reason string
}

// String formats the record as it appears in the load summary, for example
// `flight SP995: unknown origin airport "JKF"`.
func (s Skipped) String() string {
	return s.Record + ": " + s.Reason
}

// Errors for a file that cannot be used at all. Parse and Load wrap them with
// detail, so test for them with errors.Is.
var (
	// ErrUnreadable means the file could not be opened or read. The
	// underlying error is wrapped too, so errors.Is(err, fs.ErrNotExist)
	// also works.
	ErrUnreadable = errors.New("schedule cannot be read")

	// ErrInvalidJSON means the content is not well-formed JSON.
	ErrInvalidJSON = errors.New("schedule is not valid JSON")

	// ErrMissingAirports means the top level is not an object with an
	// "airports" array.
	ErrMissingAirports = errors.New(`schedule has no "airports" array`)

	// ErrMissingFlights means the top level has no "flights" array.
	ErrMissingFlights = errors.New(`schedule has no "flights" array`)

	// ErrNoValidAirports means every airport record was skipped, or there
	// were none.
	ErrNoValidAirports = errors.New("schedule has no valid airport")

	// ErrNoValidFlights means every flight record was skipped, or there were
	// none.
	ErrNoValidFlights = errors.New("schedule has no valid flight")
)

// Load reads and validates the schedule file at path. See Parse.
func Load(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	defer f.Close()

	d, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// Parse reads and validates a schedule document.
//
// A record that fails validation is skipped and listed in Dataset.Skipped;
// it never fails the whole load. Parse returns an error only when the
// document as a whole is unusable: it cannot be read, is not valid JSON,
// lacks the "airports" or "flights" array, or yields no valid airport or no
// valid flight. A service started from such a file would look healthy while
// answering every search with nothing, so the caller should refuse to start.
//
// Airports are returned in code order and flights in file order. Skipped
// lists rejected airports, then rejected flights, each in file order.
func Parse(r io.Reader) (*Dataset, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	airportRecords, flightRecords, err := splitDocument(data)
	if err != nil {
		return nil, err
	}

	var d Dataset
	known := make(map[string]flight.Airport, len(airportRecords))
	zones := make(map[string]*time.Location)
	for i, raw := range airportRecords {
		a, err := parseAirport(raw, known, zones)
		if err != nil {
			d.skip("airport", a.Code, i, err)
			continue
		}
		known[a.Code] = a
		d.Airports = append(d.Airports, a)
	}
	if len(d.Airports) == 0 {
		return nil, noneValid(ErrNoValidAirports, d.Skipped)
	}
	slices.SortFunc(d.Airports, func(a, b flight.Airport) int {
		return strings.Compare(a.Code, b.Code)
	})

	accepted := make(map[string]bool, len(flightRecords))
	firstFlightSkip := len(d.Skipped)
	for i, raw := range flightRecords {
		f, err := parseFlight(raw, known, accepted)
		if err != nil {
			d.skip("flight", f.Number, i, err)
			continue
		}
		accepted[f.Number] = true
		d.Flights = append(d.Flights, f)
	}
	if len(d.Flights) == 0 {
		return nil, noneValid(ErrNoValidFlights, d.Skipped[firstFlightSkip:])
	}
	return &d, nil
}

// splitDocument checks the top-level shape of the document and returns the
// two record arrays undecoded, so that each record can then be decoded on its
// own and a malformed one cannot take its neighbours down with it.
func splitDocument(data []byte) (airports, flights []json.RawMessage, err error) {
	if !json.Valid(data) {
		// Decode again only to obtain the position of the syntax error.
		var discard any
		return nil, nil, fmt.Errorf("%w: %w", ErrInvalidJSON, json.Unmarshal(data, &discard))
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("%w: the top level is not an object", ErrMissingAirports)
	}
	if airports, err = recordArray(doc, "airports", ErrMissingAirports); err != nil {
		return nil, nil, err
	}
	if flights, err = recordArray(doc, "flights", ErrMissingFlights); err != nil {
		return nil, nil, err
	}
	return airports, flights, nil
}

func recordArray(doc map[string]json.RawMessage, key string, missing error) ([]json.RawMessage, error) {
	raw, ok := doc[key]
	if !ok {
		return nil, missing
	}
	// A null decodes into a nil slice without error, so the kind of value has
	// to be checked before decoding.
	if !isKind(raw, '[') {
		return nil, fmt.Errorf("%w: %q is %s", missing, key, kindOf(raw))
	}
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("%w: %w", missing, err)
	}
	return records, nil
}

// noneValid builds the error for an array that yielded nothing usable, given
// the records skipped from it. It carries the first reason, which is usually
// enough for an operator to see what is wrong with the file.
func noneValid(sentinel error, skipped []Skipped) error {
	if len(skipped) == 0 {
		return fmt.Errorf("%w: the array is empty", sentinel)
	}
	return fmt.Errorf("%w: all %d records were skipped, the first as %s", sentinel, len(skipped), skipped[0])
}

func (d *Dataset) skip(kind, name string, index int, reason error) {
	label := fmt.Sprintf("%s #%d", kind, index+1)
	if name != "" {
		label = kind + " " + name
	}
	d.Skipped = append(d.Skipped, Skipped{Record: label, Reason: reason.Error()})
}
