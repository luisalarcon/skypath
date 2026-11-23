package flight

import (
	"errors"
	"fmt"
	"time"
)

// Date is a calendar date with no time of day and no time zone. A search is
// made for a Date; it only becomes a range of instants once it is anchored to
// an airport's zone with Start.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

const dateLayout = "2006-01-02"

// ErrInvalidDate is returned when text is not a real date in YYYY-MM-DD form.
var ErrInvalidDate = errors.New("invalid date")

// ParseDate reads a date in ISO 8601 calendar form, YYYY-MM-DD. Dates that do
// not exist, such as 2024-02-30, are rejected.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("%w: %q", ErrInvalidDate, s)
	}
	return DateOf(t), nil
}

// DateOf returns the calendar date of t in t's own location.
func DateOf(t time.Time) Date {
	year, month, day := t.Date()
	return Date{Year: year, Month: month, Day: day}
}

// Start returns the first instant of the date in the given location.
func (d Date) Start(loc *time.Location) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, loc)
}

// AddDays returns the date n days later (or earlier, for negative n).
func (d Date) AddDays(n int) Date {
	return DateOf(time.Date(d.Year, d.Month, d.Day+n, 0, 0, 0, 0, time.UTC))
}

// Compare returns -1, 0 or +1 when d is before, equal to or after other.
func (d Date) Compare(other Date) int {
	return d.Start(time.UTC).Compare(other.Start(time.UTC))
}

// IsZero reports whether d is the zero value.
func (d Date) IsZero() bool {
	return d == Date{}
}

// String formats the date as YYYY-MM-DD.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// MarshalText implements encoding.TextMarshaler so a Date serialises as a
// plain YYYY-MM-DD string in JSON.
func (d Date) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Date) UnmarshalText(text []byte) error {
	parsed, err := ParseDate(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
