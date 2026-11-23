# Search: time, connection rules and the algorithm

## Time model

Schedule times are local to their airport. Local times cannot be compared or subtracted directly:

| Flight | Local times | Looks like | Actually |
|---|---|---|---|
| JFK → LAX | 08:30 → 11:45 | 3h 15m | 6h 15m |
| SYD → LAX | 09:00 → 06:00, same date | −3h | 15h |
| PHX → LAX | 06:00 → 06:30 | 30m | 30m |
| LAX → SYD | 22:00 → 08:00 two days later | 34h | 16h |

So every time is converted **once, at load**, to an absolute instant that carries its airport's zone. After that:

- durations and layovers are plain subtraction of instants;
- formatting an instant yields the local time at its own airport;
- no code anywhere adds or subtracts UTC offsets by hand.

The zone rules come from the IANA time zone database, which is compiled into the binary. That matters on the schedule's date, 2024-03-15:

- the United States is already on daylight time, Europe is not yet;
- Phoenix never observes daylight time, so it matches Los Angeles in March but not in winter;
- Sydney is on daylight time and is 18 hours ahead of Los Angeles.

A fixed table of offsets would get at least one of these wrong.

**Known limit.** A local time that falls in a daylight-saving gap or overlap is ambiguous. Go resolves a gap by moving forward and an overlap by taking the first occurrence. No time in the current schedule is affected.

## Connection rules

For a flight **A** followed by a flight **B**:

| # | Rule |
|---|---|
| 1 | **Same airport.** B departs from the airport A arrives at, matched on airport code. Two airports in one city are different airports. |
| 2 | **Layover** is B's departure instant minus A's arrival instant. |
| 3 | **Kind.** The connection is *domestic* only when A's origin, the connecting airport and B's destination are all in the same country. Otherwise it is *international*. |
| 4 | **Minimum layover** is 45 minutes for a domestic connection and 90 minutes for an international one. |
| 5 | **Maximum layover** is 6 hours. |
| 6 | **Both bounds are inclusive.** A layover of exactly 45, 90 or 360 minutes is allowed. |

Rule 3 in examples:

| Route | Kind | Why |
|---|---|---|
| JFK → ORD → LAX | domestic | all three airports are in the US |
| JFK → LHR → CDG | international | neither flight is domestic |
| MIA → JFK → MEX | international | the second flight leaves the country |
| LHR → JFK → SFO | international | the first flight arrives from abroad |

An itinerary as a whole must also satisfy:

| # | Rule |
|---|---|
| 7 | **At most three flights**, which is two stops. |
| 8 | **No airport twice.** An itinerary never returns to an airport it has already been through, including the origin. |
| 9 | **Search date.** The first flight departs on the requested calendar date *in the origin airport's local time*. Later flights may depart on following days. |

## Algorithm

The flight network is a graph: airports are nodes and flights are edges with times. Because an itinerary has at most three flights, the search is a depth-limited walk that enumerates every valid path. Nothing is approximated and nothing is ranked away; the caller gets all of them.

```
Search(origin, destination, date):
    reject if origin or destination is unknown, if they are equal, or if date is missing

    dayStart = date at 00:00 in the origin's zone
    dayEnd   = the next date at 00:00 in the origin's zone

    for each flight f leaving origin at or after dayStart, while f departs before dayEnd:
        extend([f], visited = {origin, f.destination})

    sort results and return


extend(path, visited):
    last = final flight of path

    if last.destination is the search destination:
        record path as an itinerary
        return
    if path already has three flights:
        return

    earliest = last.arrival + 45 minutes      # the smaller of the two minimums
    latest   = last.arrival + 6 hours

    for each flight g leaving last.destination at or after earliest, while g departs no later than latest:
        if g.destination is in visited:        skip
        if the layover is below the minimum for this connection's kind:  skip
        extend(path + [g], visited + {g.destination})
```

Two details make this cheap:

- **The index does the filtering.** Flights from each airport are sorted by departure, so "at or after `earliest`" is a binary search and "no later than `latest`" ends the walk. Flights outside the legal window are never looked at.
- **One lookup serves both steps.** The first step and every later step ask the store the same question, "flights leaving X at or after T", with different bounds.

The lookup starts at 45 minutes because that is the earliest any connection could be legal. A connection that turns out to be international is then checked against 90.

### Ordering

Results are sorted by:

1. total travel time, shortest first;
2. total price, cheapest first;
3. departure time, earliest first;
4. flight numbers, as a final tie-break.

The fourth key makes the order fully deterministic, so the same search always returns the same list.

### Cost

Let *d* be the number of flights leaving an airport inside a six-hour window, and *F* the number leaving the origin on the search date. The walk visits at most *F · d · d* flights, each found by binary search. In the current schedule *F* is at most 36 and *d* is around ten at the busiest airports, so a search inspects a few hundred flights at most. The largest result is 27 itineraries. A benchmark in the `search` package measures the actual cost.

Memory per search is proportional to the number of itineraries returned.

### Cancellation

`Search` takes a context and stops early if the request is cancelled or times out.

## Assumptions

1. The search date applies to the first departure, in the origin's local time.
2. Layover limits are inclusive at both ends.
3. A connection is domestic only if all three airports are in one country.
4. Connections are matched on exact airport code. There is no transfer between airports in the same city.
5. An itinerary never visits an airport twice.
6. Minimum connection time depends only on domestic versus international, not on the airport or terminal.
7. An itinerary's price is the sum of its flights' prices. There are no through-fares.
8. The time zone database is trusted even where a result looks surprising, such as the 30-minute Phoenix to Los Angeles flight in the current schedule.
9. Seat availability is not modelled: every scheduled flight is bookable.
