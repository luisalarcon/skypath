package web

import (
	"slices"
	"testing"

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/view"
)

func TestSummarise(t *testing.T) {
	cases := []struct {
		name    string
		results view.Results
		want    []string
	}{
		{"mixed", view.Results{Count: 27, Direct: 3, OneStop: 14, TwoStop: 10},
			[]string{"27 itineraries", "3 direct", "14 one stop", "10 two stops"}},
		{"groups with none are left out", view.Results{Count: 13, OneStop: 4, TwoStop: 9},
			[]string{"13 itineraries", "4 one stop", "9 two stops"}},
		{"singular", view.Results{Count: 1, TwoStop: 1},
			[]string{"1 itinerary", "1 two stops"}},
		{"empty", view.Results{},
			[]string{"0 itineraries"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarise(&tc.results); !slices.Equal(got, tc.want) {
				t.Errorf("summarise = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRetryLink(t *testing.T) {
	coverage := &view.Dataset{FirstDate: "Mar 15, 2024", FirstDateISO: "2024-03-15"}
	empty := func(date string) *client.SearchResponse {
		return &client.SearchResponse{
			Origin:      client.Airport{Code: "JFK"},
			Destination: client.Airport{Code: "LAX"},
			Date:        date,
		}
	}

	t.Run("an empty search on another date offers the first covered date", func(t *testing.T) {
		link := retryLink(empty("2024-03-20"), coverage)
		if link == nil {
			t.Fatal("no link offered")
		}
		if want := "/?date=2024-03-15&destination=LAX&origin=JFK"; link.URL != want {
			t.Errorf("URL = %q, want %q", link.URL, want)
		}
		if want := "Search Mar 15, 2024 instead"; link.Label != want {
			t.Errorf("Label = %q, want %q", link.Label, want)
		}
	})

	t.Run("nothing to offer when the search was already for that date", func(t *testing.T) {
		if link := retryLink(empty("2024-03-15"), coverage); link != nil {
			t.Errorf("offered %+v for the date already searched", link)
		}
	})

	t.Run("nothing to offer when the covered dates are unknown", func(t *testing.T) {
		if link := retryLink(empty("2024-03-20"), nil); link != nil {
			t.Errorf("offered %+v without knowing the schedule", link)
		}
	})

	t.Run("nothing to offer when the search found itineraries", func(t *testing.T) {
		found := empty("2024-03-20")
		found.Itineraries = []client.Itinerary{{}}
		if link := retryLink(found, coverage); link != nil {
			t.Errorf("offered %+v alongside results", link)
		}
	})
}
