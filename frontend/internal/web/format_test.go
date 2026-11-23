package web

import (
	"testing"
	"time"
)

// at builds a time carrying a fixed UTC offset, like the API's timestamps.
func at(year int, month time.Month, day, hour, minute, offsetSeconds int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.FixedZone("", offsetSeconds))
}

const (
	hour = 3600
	half = 1800
)

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"early morning", at(2024, 3, 15, 7, 0, -4*hour), "7:00 AM"},
		{"midnight", at(2024, 3, 15, 0, 0, 0), "12:00 AM"},
		{"just after midnight", at(2024, 3, 15, 0, 5, 0), "12:05 AM"},
		{"noon", at(2024, 3, 15, 12, 0, 0), "12:00 PM"},
		{"afternoon", at(2024, 3, 15, 13, 45, 9*hour), "1:45 PM"},
		{"last minute of the day", at(2024, 3, 15, 23, 59, 0), "11:59 PM"},
		{"minutes keep their zero", at(2024, 3, 15, 9, 5, 0), "9:05 AM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTime(tt.in); got != tt.want {
				t.Errorf("formatTime = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDates(t *testing.T) {
	tests := []struct {
		name        string
		in          time.Time
		short, long string
		schedule    string
	}{
		{"documentation example", at(2024, 3, 15, 8, 0, 0), "Fri, Mar 15", "Friday, March 15, 2024", "Mar 15, 2024"},
		{"single digit day has no padding", at(2024, 3, 2, 8, 0, 0), "Sat, Mar 2", "Saturday, March 2, 2024", "Mar 2, 2024"},
		{"new year", at(2025, 1, 1, 0, 30, 9*hour), "Wed, Jan 1", "Wednesday, January 1, 2025", "Jan 1, 2025"},
		{"leap day", at(2024, 2, 29, 23, 0, -8*hour), "Thu, Feb 29", "Thursday, February 29, 2024", "Feb 29, 2024"},
		{"end of year", at(2024, 12, 31, 12, 0, 0), "Tue, Dec 31", "Tuesday, December 31, 2024", "Dec 31, 2024"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatShortDate(tt.in); got != tt.short {
				t.Errorf("formatShortDate = %q, want %q", got, tt.short)
			}
			if got := formatLongDate(tt.in); got != tt.long {
				t.Errorf("formatLongDate = %q, want %q", got, tt.long)
			}
			if got := formatScheduleDate(tt.in); got != tt.schedule {
				t.Errorf("formatScheduleDate = %q, want %q", got, tt.schedule)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		minutes int
		want    string
	}{
		{0, "0m"},
		{1, "1m"},
		{45, "45m"},
		{59, "59m"},
		{60, "1h"},
		{61, "1h 1m"},
		{120, "2h"},
		{450, "7h 30m"},
		{1260, "21h"},
		{1500, "25h"},
		{-5, "0m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatDuration(tt.minutes); got != tt.want {
				t.Errorf("formatDuration(%d) = %q, want %q", tt.minutes, got, tt.want)
			}
		})
	}
}

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{0, "$0.00"},
		{5, "$0.05"},
		{99, "$0.99"},
		{100, "$1.00"},
		{41400, "$414.00"},
		{99999, "$999.99"},
		{100000, "$1,000.00"},
		{113700, "$1,137.00"},
		{123456789, "$1,234,567.89"},
		{100000000, "$1,000,000.00"},
		{-150, "-$1.50"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatPrice(tt.cents); got != tt.want {
				t.Errorf("formatPrice(%d) = %q, want %q", tt.cents, got, tt.want)
			}
		})
	}
}

func TestPriceCents(t *testing.T) {
	tests := []struct {
		price float64
		want  int64
	}{
		{0, 0},
		{414, 41400},
		{189.5, 18950},
		{0.07, 7},
		{19.99, 1999},
		{414.00000000000006, 41400},
		{1137, 113700},
	}
	for _, tt := range tests {
		if got := priceCents(tt.price); got != tt.want {
			t.Errorf("priceCents(%v) = %d, want %d", tt.price, got, tt.want)
		}
	}
}

func TestFormatUTCOffset(t *testing.T) {
	tests := []struct {
		name   string
		offset int
		want   string
	}{
		{"zero", 0, "UTC"},
		{"new york daylight", -4 * hour, "UTC-4"},
		{"los angeles daylight", -7 * hour, "UTC-7"},
		{"tokyo", 9 * hour, "UTC+9"},
		{"india half hour", 5*hour + half, "UTC+5:30"},
		{"nepal quarter hour", 5*hour + 2700, "UTC+5:45"},
		{"newfoundland negative half hour", -(3*hour + half), "UTC-3:30"},
		{"double digit", 12 * hour, "UTC+12"},
		{"double digit negative", -10 * hour, "UTC-10"},
		{"just under an hour", half, "UTC+0:30"},
		{"negative just under an hour", -half, "UTC-0:30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUTCOffset(at(2024, 3, 15, 12, 0, tt.offset)); got != tt.want {
				t.Errorf("formatUTCOffset = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatUTCOffsetOfParsedZulu(t *testing.T) {
	// The API may write a zero offset as "Z".
	parsed, err := time.Parse(time.RFC3339, "2024-03-15T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if got := formatUTCOffset(parsed); got != "UTC" {
		t.Errorf("formatUTCOffset = %q, want UTC", got)
	}
}

func TestFormatDayOffset(t *testing.T) {
	tests := []struct {
		name      string
		departure time.Time
		arrival   time.Time
		want      string
	}{
		{"same day", at(2024, 3, 15, 8, 30, -4*hour), at(2024, 3, 15, 11, 45, -7*hour), ""},
		{"same day despite elapsed time over 24h of offsets", at(2024, 3, 15, 0, 5, 14*hour), at(2024, 3, 15, 23, 55, -12*hour), ""},
		{"next day after midnight", at(2024, 3, 15, 22, 0, -4*hour), at(2024, 3, 16, 6, 30, 0), "+1 day"},
		{"short flight over local midnight", at(2024, 3, 15, 23, 30, 0), at(2024, 3, 16, 0, 15, 0), "+1 day"},
		{"two days later across the date line", at(2024, 3, 15, 14, 0, -7*hour), at(2024, 3, 17, 3, 0, 9*hour), "+2 days"},
		{"previous day when flying east to west over the date line", at(2024, 3, 15, 1, 0, 9*hour), at(2024, 3, 14, 19, 0, -7*hour), "-1 day"},
		{"two days earlier", at(2024, 3, 17, 1, 0, 9*hour), at(2024, 3, 15, 8, 0, -7*hour), "-2 days"},
		{"three days later", at(2024, 3, 15, 8, 0, 0), at(2024, 3, 18, 8, 0, 0), "+3 days"},
		{"month boundary", at(2024, 2, 29, 20, 0, 0), at(2024, 3, 1, 4, 0, 0), "+1 day"},
		{"year boundary", at(2024, 12, 31, 20, 0, 0), at(2025, 1, 1, 4, 0, 0), "+1 day"},
		{"23:59 to 00:00 is a new day", at(2024, 3, 15, 23, 59, 0), at(2024, 3, 16, 0, 0, 0), "+1 day"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDayOffset(tt.departure, tt.arrival); got != tt.want {
				t.Errorf("formatDayOffset = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatStops(t *testing.T) {
	tests := []struct {
		stops int
		want  string
	}{
		{0, "Direct"},
		{1, "1 stop"},
		{2, "2 stops"},
		{3, "3 stops"},
	}
	for _, tt := range tests {
		if got := formatStops(tt.stops); got != tt.want {
			t.Errorf("formatStops(%d) = %q, want %q", tt.stops, got, tt.want)
		}
	}
}

func TestFormatLayover(t *testing.T) {
	tests := []struct {
		apiType   string
		wantKind  string
		wantLabel string
	}{
		{"domestic", "domestic", "Domestic connection"},
		{"international", "international", "International connection"},
		{"", "international", "International connection"},
		{"unexpected", "international", "International connection"},
	}
	for _, tt := range tests {
		t.Run(tt.apiType, func(t *testing.T) {
			kind, label := formatLayover(tt.apiType)
			if kind != tt.wantKind || label != tt.wantLabel {
				t.Errorf("formatLayover(%q) = %q, %q; want %q, %q", tt.apiType, kind, label, tt.wantKind, tt.wantLabel)
			}
		})
	}
}
