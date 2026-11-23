// Package dataset reads the flight schedule file and turns it into validated
// domain values.
//
// Loading is where raw input is made trustworthy: local times become absolute
// instants in their airport's time zone, prices become exact cents, and any
// record that cannot be trusted is skipped with a recorded reason instead of
// being repaired or silently dropped. Everything downstream can then rely on
// the data without re-checking it. The rules, and the order in which they are
// checked, are those of docs/data.md.
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
// Load and Parse fail only when the file as a whole is unusable: it cannot be
// read, is not valid JSON, lacks the "airports" or "flights" array, or
// contains no valid airport or no valid flight. Each case has a sentinel
// error (ErrUnreadable and so on) to test for with errors.Is.
//
// The package resolves time zones with time.LoadLocation, so the program
// should embed the zone database (import _ "time/tzdata") to behave the same
// on every host.
package dataset
