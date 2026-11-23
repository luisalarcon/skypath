// Package dataset reads the flight schedule file and turns it into validated
// domain values.
//
// Loading is where raw input is made trustworthy: local times become absolute
// instants in their airport's time zone, prices become exact cents, and any
// record that cannot be trusted is skipped with a recorded reason instead of
// being repaired or silently dropped. Everything downstream can then rely on
// the data without re-checking it.
//
// Public API:
//
//	Load(path string) (*Dataset, error)
//	Parse(r io.Reader) (*Dataset, error)
//
//	type Dataset struct {
//		Airports []flight.Airport // ordered by code
//		Flights  []flight.Flight  // every valid flight, in file order
//		Skipped  []Skipped        // rejected records, in file order
//	}
//
//	type Skipped struct {
//		Record string // e.g. "flight SP995" or "airport #3"
//		Reason string // plain language, e.g. `unknown origin airport "JKF"`
//	}
//
// Load fails only when the file as a whole is unusable: it cannot be read, is
// not valid JSON, or contains no valid airports or flights.
package dataset
