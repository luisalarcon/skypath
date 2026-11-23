package web

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/luisalarcon/skypath/frontend/internal/client"
)

// The functions in this file are the formatting table in docs/frontend.md,
// one function per row. They are pure so the table can be tested directly.
//
// Times arrive from the API as RFC 3339 values that carry their airport's UTC
// offset. Each function formats in the offset the value carries and never
// converts, so a time always reads as the wall clock at its own airport.

// formatTime renders a time of day: "7:00 AM", "12:00 PM".
func formatTime(t time.Time) string {
	return t.Format("3:04 PM")
}

// formatShortDate renders a date as weekday, month and day: "Fri, Mar 15".
func formatShortDate(t time.Time) string {
	return t.Format("Mon, Jan 2")
}

// formatLongDate renders a date in full: "Friday, March 15, 2024".
func formatLongDate(t time.Time) string {
	return t.Format("Monday, January 2, 2006")
}

// formatScheduleDate renders a date as month, day and year: "Mar 15, 2024".
func formatScheduleDate(t time.Time) string {
	return t.Format("Jan 2, 2006")
}

// formatDuration renders whole minutes as hours and minutes, leaving out a
// zero part: "45m", "2h", "7h 30m". Durations never roll over into days,
// because "21h" reads better than "0d 21h" for a journey of that length.
func formatDuration(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	h, m := minutes/60, minutes%60
	switch {
	case h == 0:
		return strconv.Itoa(m) + "m"
	case m == 0:
		return strconv.Itoa(h) + "h"
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// priceCents converts an API price to whole cents. Prices have two decimals,
// so rounding only removes floating-point noise such as 414.00000000000006.
func priceCents(price float64) int64 {
	return int64(math.Round(price * 100))
}

// formatPrice renders cents as dollars with thousands separators:
// "$414.00", "$1,137.00". The API's currency is USD throughout; the symbol
// is fixed here until the API offers another.
func formatPrice(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}

	dollars := strconv.FormatInt(cents/100, 10)
	var grouped strings.Builder
	for i, digit := range dollars {
		if i > 0 && (len(dollars)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return fmt.Sprintf("%s$%s.%02d", sign, grouped.String(), cents%100)
}

// formatUTCOffset renders the offset a time carries: "UTC-4", "UTC+9",
// "UTC+5:30", "UTC". Minutes appear only when the offset has them.
func formatUTCOffset(t time.Time) string {
	_, seconds := t.Zone()
	if seconds == 0 {
		return "UTC"
	}

	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hours, minutes := seconds/3600, seconds%3600/60
	if minutes == 0 {
		return fmt.Sprintf("UTC%s%d", sign, hours)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, hours, minutes)
}

// formatDayOffset marks an arrival that falls on a different calendar date
// from its departure: "+1 day", "+2 days", "-1 day", or "" for the same day.
// It compares the calendar dates of the two local times, not the elapsed
// time: a 90 minute flight that lands after local midnight arrives "+1 day".
func formatDayOffset(departure, arrival time.Time) string {
	days := calendarDaysBetween(departure, arrival)
	switch {
	case days == 0:
		return ""
	case days == 1 || days == -1:
		return fmt.Sprintf("%+d day", days)
	default:
		return fmt.Sprintf("%+d days", days)
	}
}

// calendarDaysBetween counts the calendar days from a's date to b's date,
// each read in its own offset.
func calendarDaysBetween(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	from := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	to := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(to.Sub(from) / (24 * time.Hour))
}

// formatStops renders the number of connections: "Direct", "1 stop",
// "2 stops".
func formatStops(stops int) string {
	switch stops {
	case 0:
		return "Direct"
	case 1:
		return "1 stop"
	default:
		return fmt.Sprintf("%d stops", stops)
	}
}

// formatLayover returns the kind and the label of a connection. Anything the
// API does not call domestic is treated as international, the stricter case.
func formatLayover(apiType string) (kind, label string) {
	if apiType == client.LayoverDomestic {
		return "domestic", "Domestic connection"
	}
	return "international", "International connection"
}
