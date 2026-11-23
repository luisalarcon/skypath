package dataset

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// parseAirport validates one airport record, applying the checks in the
// order docs/data.md lists them so the first failure is the reason given.
//
// known holds the airports accepted so far and zones caches loaded time
// zones by name. On failure the returned airport carries its code whenever
// the code itself was valid, so the record can be labelled by it.
func parseAirport(raw json.RawMessage, known map[string]flight.Airport, zones map[string]*time.Location) (flight.Airport, error) {
	r, err := decodeRecord(raw)
	if err != nil {
		return flight.Airport{}, err
	}

	given, err := r.requiredText("code", "code")
	if err != nil {
		return flight.Airport{}, err
	}
	code := flight.NormalizeCode(given)
	if !flight.ValidCode(code) {
		return flight.Airport{}, fmt.Errorf("invalid code %q: want three letters A-Z", given)
	}
	a := flight.Airport{Code: code}

	// Only accepted airports claim a code, so a valid record still loads
	// when an earlier one with the same code was skipped.
	if _, dup := known[code]; dup {
		return a, fmt.Errorf("duplicate airport code %q: the first airport with this code is kept", code)
	}

	if a.Name, err = r.requiredText("name", "name"); err != nil {
		return a, err
	}
	if a.City, err = r.requiredText("city", "city"); err != nil {
		return a, err
	}

	country, err := r.requiredText("country", "country")
	if err != nil {
		return a, err
	}
	a.Country = strings.ToUpper(country)
	if !twoLetters(a.Country) {
		return a, fmt.Errorf("invalid country %q: want two letters", country)
	}

	zone, err := r.requiredText("timezone", "time zone")
	if err != nil {
		return a, err
	}
	if a.Location, err = loadZone(zone, zones); err != nil {
		return a, err
	}
	return a, nil
}

// loadZone resolves a time zone name, caching the result so airports that
// share a zone share one *time.Location.
func loadZone(name string, cache map[string]*time.Location) (*time.Location, error) {
	if loc, ok := cache[name]; ok {
		return loc, nil
	}
	// time.LoadLocation maps "Local" to the server's own zone, which would
	// make flight times depend on where the service runs. ("" maps to UTC
	// and is already rejected as empty.)
	if name == "Local" {
		return nil, fmt.Errorf("time zone %q is not allowed: name the airport's IANA zone", name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q", name)
	}
	cache[name] = loc
	return loc, nil
}

func twoLetters(s string) bool {
	return len(s) == 2 && isUpperLetter(s[0]) && isUpperLetter(s[1])
}

func isUpperLetter(b byte) bool {
	return 'A' <= b && b <= 'Z'
}
