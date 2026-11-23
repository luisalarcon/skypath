package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

func TestSearchStopsWhenTheContextIsDone(t *testing.T) {
	src := sourceOf(tiedFlights()...)
	q := query(jfk, lax, searchDate)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()

	cases := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"cancelled", cancelled, context.Canceled},
		{"deadline passed", expired, context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewEngine(src).Search(tc.ctx, q)
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
			if got != nil {
				t.Errorf("itineraries = %q, want nil", keys(got))
			}
		})
	}
}

// cancellingSource cancels a context the first time departures are
// requested and counts every request, so a test can see whether the search
// kept going after cancellation.
type cancellingSource struct {
	FlightSource
	cancel context.CancelFunc
	calls  int
}

func (s *cancellingSource) DeparturesFrom(code string, notBefore time.Time) []flight.Flight {
	s.calls++
	s.cancel()
	return s.FlightSource.DeparturesFrom(code, notBefore)
}

func TestSearchStopsWhenCancelledPartway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := &cancellingSource{FlightSource: sourceOf(tiedFlights()...), cancel: cancel}

	got, err := NewEngine(src).Search(ctx, query(jfk, lax, searchDate))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got != nil {
		t.Errorf("itineraries = %q, want nil", keys(got))
	}
	// The first request lists the origin's departures; without cancellation
	// the walk would go on to ask for connections at ORD and DEN.
	if src.calls != 1 {
		t.Errorf("source was asked %d times, want 1: the search kept going after cancellation", src.calls)
	}
}
