package search

import (
	"reflect"
	"slices"
	"testing"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

func TestSearchOrdering(t *testing.T) {
	cases := []struct {
		name    string
		flights []flight.Flight
		want    []string
	}{
		{
			name: "shorter total travel time first, even when dearer",
			flights: []flight.Flight{
				newFlight("SP2", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 100), // 7h30m in total, 200.00
				newFlight("SP3", ord, lax, "2024-03-15 09:15", "2024-03-15 11:30", 100),
				newFlight("SP1", jfk, lax, "2024-03-15 08:30", "2024-03-15 11:45", 500), // 6h15m, 500.00
			},
			want: []string{"SP1", "SP2+SP3"},
		},
		{
			name: "equal travel time: cheaper first, even when later",
			flights: []flight.Flight{
				newFlight("SP10", jfk, lax, "2024-03-15 08:00", "2024-03-15 11:15", 300),
				newFlight("SP11", jfk, lax, "2024-03-15 12:00", "2024-03-15 15:15", 250),
			},
			want: []string{"SP11", "SP10"},
		},
		{
			name: "equal travel time and price: earlier departure first",
			flights: []flight.Flight{
				newFlight("SP20", jfk, lax, "2024-03-15 12:00", "2024-03-15 15:15", 300),
				newFlight("SP21", jfk, lax, "2024-03-15 08:00", "2024-03-15 11:15", 300),
			},
			want: []string{"SP21", "SP20"},
		},
		{
			name: "full tie on a direct flight: flight numbers",
			flights: []flight.Flight{
				newFlight("SP31", jfk, lax, "2024-03-15 08:00", "2024-03-15 11:15", 300),
				newFlight("SP30", jfk, lax, "2024-03-15 08:00", "2024-03-15 11:15", 300),
			},
			want: []string{"SP30", "SP31"},
		},
		{
			name: "full tie decided by a later segment",
			flights: []flight.Flight{
				newFlight("SP40", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 100),
				newFlight("SP42", ord, lax, "2024-03-15 09:30", "2024-03-15 11:45", 100),
				newFlight("SP41", ord, lax, "2024-03-15 09:30", "2024-03-15 11:45", 100),
			},
			want: []string{"SP40+SP41", "SP40+SP42"},
		},
		{
			name: "full tie decided by the first differing segment",
			flights: []flight.Flight{
				newFlight("SP52", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 100),
				newFlight("SP51", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 100),
				newFlight("SP50", ord, lax, "2024-03-15 09:30", "2024-03-15 11:45", 100),
				newFlight("SP59", ord, lax, "2024-03-15 09:30", "2024-03-15 11:45", 100),
			},
			want: []string{"SP51+SP50", "SP51+SP59", "SP52+SP50", "SP52+SP59"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := keys(mustSearch(t, sourceOf(tc.flights...), query(jfk, lax, searchDate)))
			if !slices.Equal(got, tc.want) {
				t.Errorf("order = %q, want %q", got, tc.want)
			}
		})
	}
}

// tiedFlights is a small network full of exact ties: flights that leave at
// the same instant, cost the same and take the same time.
func tiedFlights() []flight.Flight {
	return []flight.Flight{
		newFlight("SP31", jfk, lax, "2024-03-15 08:00", "2024-03-15 11:15", 300),
		newFlight("SP30", jfk, lax, "2024-03-15 08:00", "2024-03-15 11:15", 300),
		newFlight("SP40", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 150),
		newFlight("SP43", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 150),
		newFlight("SP42", ord, lax, "2024-03-15 09:30", "2024-03-15 11:45", 150),
		newFlight("SP41", ord, lax, "2024-03-15 09:30", "2024-03-15 11:45", 150),
		newFlight("SP44", ord, den, "2024-03-15 09:30", "2024-03-15 11:00", 75),
		newFlight("SP45", den, lax, "2024-03-15 12:00", "2024-03-15 13:15", 75),
	}
}

func TestSearchIsDeterministic(t *testing.T) {
	flights := tiedFlights()
	q := query(jfk, lax, searchDate)

	want := mustSearch(t, sourceOf(flights...), q)
	if len(want) < 6 {
		t.Fatalf("fixture produced %d itineraries, want at least 6 to make ties matter", len(want))
	}

	// Repeated calls on one source.
	src := sourceOf(flights...)
	for range 5 {
		if got := mustSearch(t, src, q); !reflect.DeepEqual(got, want) {
			t.Fatalf("repeated search order = %q, want %q", keys(got), keys(want))
		}
	}

	// The same flights inserted in reverse, so every tied group comes back
	// from the source in the opposite order.
	reversed := slices.Clone(flights)
	slices.Reverse(reversed)
	if got := mustSearch(t, sourceOf(reversed...), q); !reflect.DeepEqual(got, want) {
		t.Errorf("order with reversed insertion = %q, want %q", keys(got), keys(want))
	}
}
