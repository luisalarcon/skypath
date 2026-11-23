// Package flight holds the domain model shared by every backend package:
// airports, flights, itineraries and the value types they are built from.
//
// It has no dependencies on other SkyPath packages. Values are immutable by
// convention: once the dataset is loaded nothing modifies them, which is what
// allows the rest of the service to share them across requests without locks.
package flight

import (
	"strings"
	"time"
)

// Airport is a commercial airport identified by its IATA code.
type Airport struct {
	Code    string // IATA code, three upper-case letters, e.g. "JFK"
	Name    string
	City    string
	Country string // ISO 3166-1 alpha-2, e.g. "US"

	// Location is the airport's IANA time zone. Every local time at this
	// airport is interpreted in it.
	Location *time.Location
}

// NormalizeCode trims surrounding space and upper-cases an airport code so
// that user input such as " jfk " compares equal to "JFK".
func NormalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// ValidCode reports whether s has the shape of an IATA airport code: exactly
// three upper-case ASCII letters. It says nothing about whether the airport
// exists.
func ValidCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}
