package flight

import (
	"errors"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	got, err := ParseDate("2024-03-15")
	if err != nil {
		t.Fatalf("ParseDate returned error: %v", err)
	}
	if want := (Date{Year: 2024, Month: time.March, Day: 15}); got != want {
		t.Errorf("ParseDate = %+v, want %+v", got, want)
	}

	invalid := []string{"", "2024-3-15", "15-03-2024", "2024/03/15", "2024-02-30", "2024-13-01", "2024-03-15T00:00:00", "tomorrow"}
	for _, in := range invalid {
		if _, err := ParseDate(in); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseDate(%q) error = %v, want ErrInvalidDate", in, err)
		}
	}
}

func TestDateString(t *testing.T) {
	d := Date{Year: 2024, Month: time.March, Day: 5}
	if got := d.String(); got != "2024-03-05" {
		t.Errorf("String = %q, want 2024-03-05", got)
	}
}

func TestDateAddDays(t *testing.T) {
	cases := []struct {
		start Date
		days  int
		want  string
	}{
		{Date{2024, time.March, 15}, 1, "2024-03-16"},
		{Date{2024, time.February, 28}, 1, "2024-02-29"}, // leap year
		{Date{2024, time.December, 31}, 1, "2025-01-01"},
		{Date{2024, time.March, 1}, -1, "2024-02-29"},
	}
	for _, tc := range cases {
		if got := tc.start.AddDays(tc.days).String(); got != tc.want {
			t.Errorf("%s + %d days = %s, want %s", tc.start, tc.days, got, tc.want)
		}
	}
}

func TestDateStartIsLocalMidnight(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	d := Date{Year: 2024, Month: time.March, Day: 15}

	// The same calendar date starts at different instants in different zones:
	// New York is UTC-4 and Sydney UTC+11 on this date.
	if got, want := d.Start(newYork).UTC(), time.Date(2024, time.March, 15, 4, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("New York start = %s, want %s", got, want)
	}
	if got, want := d.Start(sydney).UTC(), time.Date(2024, time.March, 14, 13, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Sydney start = %s, want %s", got, want)
	}
}

func TestDateOfUsesTheInstantsOwnZone(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	// 20:00 UTC on the 15th is already the 16th in Tokyo.
	instant := time.Date(2024, time.March, 15, 20, 0, 0, 0, time.UTC)
	if got := DateOf(instant).String(); got != "2024-03-15" {
		t.Errorf("DateOf(UTC) = %s, want 2024-03-15", got)
	}
	if got := DateOf(instant.In(tokyo)).String(); got != "2024-03-16" {
		t.Errorf("DateOf(Tokyo) = %s, want 2024-03-16", got)
	}
}

func TestDateCompare(t *testing.T) {
	early := Date{2024, time.March, 15}
	late := Date{2024, time.March, 16}
	if early.Compare(late) != -1 || late.Compare(early) != 1 || early.Compare(early) != 0 {
		t.Errorf("Compare ordering is wrong")
	}
	if !(Date{}).IsZero() || early.IsZero() {
		t.Errorf("IsZero is wrong")
	}
}
