package search

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// networkAirports are 25 real airports spread over eighteen time zones on
// five continents, about the size of the production schedule.
var networkAirports = []flight.Airport{
	jfk, bos, mia, ord, den, phx, lax, sfo, sea,
	newAirport("ATL", "Atlanta", "US", "America/New_York"),
	newAirport("DFW", "Dallas", "US", "America/Chicago"),
	newAirport("IAH", "Houston", "US", "America/Chicago"),
	newAirport("YYZ", "Toronto", "CA", "America/Toronto"),
	mex,
	newAirport("GRU", "Sao Paulo", "BR", "America/Sao_Paulo"),
	lhr, cdg,
	newAirport("FRA", "Frankfurt", "DE", "Europe/Berlin"),
	newAirport("AMS", "Amsterdam", "NL", "Europe/Amsterdam"),
	newAirport("MAD", "Madrid", "ES", "Europe/Madrid"),
	newAirport("DXB", "Dubai", "AE", "Asia/Dubai"),
	newAirport("SIN", "Singapore", "SG", "Asia/Singapore"),
	nrt,
	newAirport("HND", "Tokyo", "JP", "Asia/Tokyo"),
	syd,
}

// generateNetwork returns a reproducible random schedule over
// networkAirports: perDay flights on each of days consecutive dates centred
// on searchDate. Departures fall on five-minute marks at any hour of the
// origin's local day, so many flights tie and many sit near midnight.
func generateNetwork(seed uint64, perDay, days int) []flight.Flight {
	rng := rand.New(rand.NewPCG(seed, seed))
	n := len(networkAirports)
	flights := make([]flight.Flight, 0, perDay*days)
	for i := range perDay * days {
		from := rng.IntN(n)
		to := rng.IntN(n - 1)
		if to >= from {
			to++
		}
		origin, destination := networkAirports[from], networkAirports[to]

		date := searchDate.AddDays(i/perDay - (days-1)/2)
		departure := date.Start(origin.Location).Add(time.Duration(rng.IntN(24*12)*5) * time.Minute)
		duration := time.Duration(60+rng.IntN(108)*5) * time.Minute // 1h to 9h55m

		flights = append(flights, flight.Flight{
			Number:      fmt.Sprintf("SP%04d", i),
			Airline:     "SkyPath",
			Origin:      origin,
			Destination: destination,
			Departure:   departure,
			Arrival:     departure.Add(duration).In(destination.Location),
			Price:       flight.Money((80 + rng.IntN(800)) * 100),
		})
	}
	return flights
}

// allQueries returns a query for every ordered pair of distinct airports.
func allQueries(airports []flight.Airport, date flight.Date) []flight.Query {
	var queries []flight.Query
	for _, from := range airports {
		for _, to := range airports {
			if from.Code != to.Code {
				queries = append(queries, query(from, to, date))
			}
		}
	}
	return queries
}

// exhaustiveSearch answers q by trying every chain of up to three flights in
// the schedule: no index, no time windows, no pruning. It shares only
// connect with the engine, and connect's rules are tested on their own. It
// returns the itinerary keys, sorted.
func exhaustiveSearch(flights []flight.Flight, origin flight.Airport, q flight.Query) []string {
	start := q.Date.Start(origin.Location)
	end := q.Date.AddDays(1).Start(origin.Location)
	distinct := func(codes ...string) bool {
		return len(slices.Compact(slices.Sorted(slices.Values(codes)))) == len(codes)
	}

	var found []string
	for _, a := range flights {
		if a.Origin.Code != q.Origin || a.Departure.Before(start) || !a.Departure.Before(end) {
			continue
		}
		if a.Destination.Code == q.Destination {
			found = append(found, a.Number)
			continue
		}
		for _, b := range flights {
			if _, ok := connect(a, b); !ok || !distinct(q.Origin, a.Destination.Code, b.Destination.Code) {
				continue
			}
			if b.Destination.Code == q.Destination {
				found = append(found, a.Number+"+"+b.Number)
				continue
			}
			for _, c := range flights {
				if _, ok := connect(b, c); !ok || c.Destination.Code != q.Destination ||
					!distinct(q.Origin, a.Destination.Code, b.Destination.Code, c.Destination.Code) {
					continue
				}
				found = append(found, a.Number+"+"+b.Number+"+"+c.Number)
			}
		}
	}
	slices.Sort(found)
	return found
}

func TestSearchMatchesExhaustiveEnumeration(t *testing.T) {
	// Three days of flights, so first flights just outside the search date
	// exist and connections run into the next day.
	flights := generateNetwork(7, 150, 3)
	src := newSource(networkAirports, flights)

	byStops := make([]int, maxFlights)
	for _, q := range allQueries(networkAirports, searchDate) {
		origin, _ := src.Airport(q.Origin)
		got := mustSearch(t, src, q)
		if !slices.IsSortedFunc(got, compareItineraries) {
			t.Errorf("%s-%s: results are not in order: %q", q.Origin, q.Destination, keys(got))
		}

		gotKeys := slices.Sorted(slices.Values(keys(got)))
		if want := exhaustiveSearch(flights, origin, q); !slices.Equal(gotKeys, want) {
			t.Errorf("%s-%s: engine found %q, exhaustive enumeration found %q", q.Origin, q.Destination, gotKeys, want)
		}
		for _, it := range got {
			byStops[it.Stops()]++
		}
	}

	// The comparison only means something if every shape occurs.
	t.Logf("itineraries by stops: %v", byStops)
	for stops, count := range byStops {
		if count == 0 {
			t.Errorf("generated network produced no itineraries with %d stops", stops)
		}
	}
}

func TestSearchIgnoresSourceInsertionOrder(t *testing.T) {
	flights := generateNetwork(11, 300, 1)
	queries := allQueries(networkAirports, searchDate)
	want := make([][]string, len(queries))
	src := newSource(networkAirports, flights)
	for i, q := range queries {
		want[i] = keys(mustSearch(t, src, q))
	}

	rng := rand.New(rand.NewPCG(3, 3))
	for round := range 3 {
		shuffled := slices.Clone(flights)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		src := newSource(networkAirports, shuffled)
		for i, q := range queries {
			if got := keys(mustSearch(t, src, q)); !slices.Equal(got, want[i]) {
				t.Fatalf("shuffle %d, %s-%s: order = %q, want %q", round, q.Origin, q.Destination, got, want[i])
			}
		}
	}
}

func BenchmarkSearch(b *testing.B) {
	src := newSource(networkAirports, generateNetwork(1, 300, 1))
	engine := NewEngine(src)
	queries := allQueries(networkAirports, searchDate)
	ctx := context.Background()

	b.ReportAllocs()
	searches, found := 0, 0
	for b.Loop() {
		its, err := engine.Search(ctx, queries[searches%len(queries)])
		if err != nil {
			b.Fatal(err)
		}
		searches++
		found += len(its)
	}
	b.ReportMetric(float64(found)/float64(searches), "itineraries/op")
}

func TestReturnedItinerariesDoNotShareStorage(t *testing.T) {
	// Several itineraries here start with the same flight, which is exactly
	// when a walk that reuses buffers could hand out overlapping slices.
	got := mustSearch(t, sourceOf(tiedFlights()...), query(jfk, lax, searchDate))
	if len(got) < 2 {
		t.Fatalf("fixture produced %d itineraries, want several", len(got))
	}

	// Write a mark into every element each itinerary could reach, spare
	// capacity included, then check that each still holds only its own.
	for i, it := range got {
		mark := fmt.Sprintf("itinerary %d", i)
		segments := it.Segments[:cap(it.Segments)]
		for j := range segments {
			segments[j].Number = mark
		}
		layovers := it.Layovers[:cap(it.Layovers)]
		for j := range layovers {
			layovers[j].Airport.Code = mark
		}
	}
	for i, it := range got {
		mark := fmt.Sprintf("itinerary %d", i)
		for _, segment := range it.Segments {
			if segment.Number != mark {
				t.Errorf("itinerary %d has a segment marked %q: it shares storage", i, segment.Number)
			}
		}
		for _, layover := range it.Layovers {
			if layover.Airport.Code != mark {
				t.Errorf("itinerary %d has a layover marked %q: it shares storage", i, layover.Airport.Code)
			}
		}
	}
}

func TestEngineIsSafeForConcurrentUse(t *testing.T) {
	src := newSource(networkAirports, generateNetwork(5, 300, 1))
	engine := NewEngine(src)
	queries := allQueries(networkAirports, searchDate)

	want := make([][]string, len(queries))
	for i, q := range queries {
		want[i] = keys(mustSearch(t, src, q))
	}

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			// Each worker starts at a different query so that different
			// searches really do overlap.
			for n := range queries {
				i := (n + worker*len(queries)/8) % len(queries)
				got, err := engine.Search(context.Background(), queries[i])
				if err != nil {
					t.Errorf("worker %d, %s-%s: %v", worker, queries[i].Origin, queries[i].Destination, err)
					return
				}
				if !slices.Equal(keys(got), want[i]) {
					t.Errorf("worker %d, %s-%s: order = %q, want %q", worker, queries[i].Origin, queries[i].Destination, keys(got), want[i])
					return
				}
			}
		})
	}
	wg.Wait()
}
