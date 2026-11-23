package search

import (
	"testing"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

func TestConnectionKind(t *testing.T) {
	cases := []struct {
		name          string
		first, second flight.Flight
		want          flight.ConnectionKind
	}{
		{
			name:   "all three airports in one country",
			first:  newFlight("SP1", jfk, ord, "2024-03-15 07:00", "2024-03-15 08:30", 100),
			second: newFlight("SP2", ord, lax, "2024-03-15 10:00", "2024-03-15 12:15", 100),
			want:   flight.Domestic,
		},
		{
			name:   "neither flight domestic",
			first:  newFlight("SP1", jfk, lhr, "2024-03-15 18:00", "2024-03-16 06:00", 100),
			second: newFlight("SP2", lhr, cdg, "2024-03-16 08:00", "2024-03-16 10:15", 100),
			want:   flight.International,
		},
		{
			name:   "domestic flight feeding an international one",
			first:  newFlight("SP1", mia, jfk, "2024-03-15 07:00", "2024-03-15 10:00", 100),
			second: newFlight("SP2", jfk, mex, "2024-03-15 12:00", "2024-03-15 15:30", 100),
			want:   flight.International,
		},
		{
			name:   "international flight feeding a domestic one",
			first:  newFlight("SP1", lhr, jfk, "2024-03-15 09:00", "2024-03-15 12:00", 100),
			second: newFlight("SP2", jfk, sfo, "2024-03-15 14:00", "2024-03-15 17:30", 100),
			want:   flight.International,
		},
		{
			name:   "out and back into the same foreign country",
			first:  newFlight("SP1", lhr, jfk, "2024-03-15 09:00", "2024-03-15 12:00", 100),
			second: newFlight("SP2", jfk, lhr, "2024-03-15 18:00", "2024-03-16 06:00", 100),
			want:   flight.International,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := connectionKind(tc.first, tc.second); got != tc.want {
				t.Errorf("connectionKind(%s %s-%s, %s %s-%s) = %s, want %s",
					tc.first.Number, tc.first.Origin.Code, tc.first.Destination.Code,
					tc.second.Number, tc.second.Origin.Code, tc.second.Destination.Code,
					got, tc.want)
			}
		})
	}
}
