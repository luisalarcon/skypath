# Data: the schedule, validation and storage

## The schedule file

`flights.json` holds two arrays.

```json
{
  "airports": [
    {"code": "JFK", "name": "John F. Kennedy International", "city": "New York",
     "country": "US", "timezone": "America/New_York"}
  ],
  "flights": [
    {"flightNumber": "SP101", "airline": "SkyPath Airways", "origin": "JFK",
     "destination": "LAX", "departureTime": "2024-03-15T08:30:00",
     "arrivalTime": "2024-03-15T11:45:00", "price": 299.00, "aircraft": "A320"}
  ]
}
```

Times are **local to their airport** and carry no offset. `departureTime` is in the origin's zone and `arrivalTime` in the destination's.

**Profile of the current file**

- 25 airports in 12 countries; 13 are in the US.
- 303 flight records, one airline, 132 distinct routes among the valid records.
- Of the 303 records, 301 depart on 2024-03-15 and 2 on 2024-03-16. Arrivals run to 2024-03-17.
- Two cities have more than one airport: New York (JFK, LGA, EWR) and Tokyo (NRT, HND).

**Irregularities found in it**

| Record | Problem | Handling |
|---|---|---|
| SP995 | Origin is `JKF`, which is not an airport in the file | Skipped and reported |
| SP995, SP996, SP998 | `price` is a string (`"289.00"`, `"99"`, `"95.00"`) | Accepted; parsed as a decimal |
| SP997 and others | `price` is an integer (`109`) | Accepted |

The loader does not guess that `JKF` means `JFK`. A wrong guess would invent a flight that may not exist; a skipped record is visible and can be corrected at the source.

## Loading

Loading happens once, at startup, in five steps.

1. Read and decode the file.
2. Validate each airport and load its time zone.
3. Validate each flight and convert it to domain values.
4. Build the in-memory indexes.
5. Log a summary: airports loaded, flights loaded, records skipped and why.

The guiding rule is **parse once, then trust**. After loading, every `flight.Flight` in memory is known to be valid, so nothing downstream re-checks it.

### Airport validation

Checked in this order. The first failure is the recorded reason.

| Check | Rule |
|---|---|
| `code` | Required. After trimming and upper-casing, exactly three letters A–Z. |
| Duplicate | The first airport with a given code wins; later ones are skipped. |
| `name`, `city` | Required, non-empty after trimming. |
| `country` | Required. Two letters, stored upper-case. |
| `timezone` | Required, non-empty, and must be a zone the time zone database knows. An empty string and `Local` are rejected explicitly. |

### Flight validation

Checked in this order. The first failure is the recorded reason.

| Check | Rule |
|---|---|
| `flightNumber` | Required, non-empty after trimming. |
| Duplicate | The first flight with a given number wins; later ones are skipped. |
| `origin`, `destination` | Trimmed and upper-cased. Each must be a loaded airport. |
| Distinct endpoints | Origin and destination must differ. |
| `departureTime`, `arrivalTime` | Exactly `YYYY-MM-DDTHH:MM:SS`, read in the airport's own zone. A time that does not exist there, inside a daylight-saving gap, is rejected rather than moved. |
| Order | The arrival instant must be after the departure instant. |
| `price` | A JSON number or a string holding a decimal. Non-negative, at most two decimal places. |
| `airline`, `aircraft` | Optional text, kept as given. |

Unknown extra fields are ignored. A record that is not a JSON object is skipped.

### What is skipped and what is fatal

A bad **record** is skipped. It is listed with a label and a plain-language reason, airports first and then flights, each in file order:

```
flight SP995: unknown origin airport "JKF"
```

The label is `flight <number>` or `airport <code>` when that field is usable, otherwise the record's position, such as `flight #12`.

A bad **file** stops startup with a clear error. That means the file cannot be read, is not valid JSON, lacks the `airports` or `flights` array, or contains no valid airport or no valid flight. A service that started with no data would look healthy and answer every search with nothing.

### Normalisation

| Raw value | Becomes |
|---|---|
| Airport `timezone` name | `*time.Location` |
| Local time string | `time.Time`: an absolute instant carrying its airport's zone |
| `price` as number or string | `flight.Money`: whole cents in an integer |
| Airport codes | Upper-case, with the full airport record attached to the flight |

Prices are read from their text form directly into cents. They never pass through floating point, so `0.10 + 0.20` is exactly `0.30`.

## Storage

The data set is small, read-only and loaded once, so it lives in memory behind a narrow interface.

### Indexes

| Question | Structure | Cost |
|---|---|---|
| Is this a known airport? What are its country and zone? | `map[code]Airport` | one lookup |
| Which flights leave airport X at or after instant T? | `map[code][]Flight`, each list sorted by departure | binary search, then a forward walk |
| What airports exist? | airport list sorted by code | already built |

The second row is the only operation the search needs. "Flights leaving JFK on 15 March" and "flights leaving ORD within six hours of landing" are the same question with different bounds.

Each per-airport list is sorted by departure instant, then flight number, so results are deterministic.

`DeparturesFrom` returns a window onto the stored list rather than a copy. It allocates nothing, and its capacity is clipped so that appending to it cannot overwrite stored data.

### Concurrency

The store is built once and never modified. Any number of requests can read it at the same time with no locking, and there is no cache to invalidate.

### Why memory rather than a database

- The whole schedule is 68 KB on disk and stays well under a megabyte once indexed in memory.
- A database would add a network hop to every search, a second thing to start, and a failure mode at startup, in exchange for nothing at this size.
- The service stays a single self-contained process, which makes it trivial to run, test and deploy.

The search does not know the data is in memory. It depends on a two-method interface:

```go
type FlightSource interface {
    Airport(code string) (flight.Airport, bool)
    DeparturesFrom(code string, notBefore time.Time) []flight.Flight
}
```

### Growing beyond memory

What would change, in the order it would be needed:

1. **Many days of schedule.** Partition the index by departure date so a search touches one or two days.
2. **Data that changes.** Build a new store in the background and swap a pointer, so readers never see a partial update.
3. **Data larger than memory.** Implement `FlightSource` over a database with an index on `(origin, departure)`; the range query is identical.
4. **Heavy repeated traffic.** Cache results by `(origin, destination, date)`; entries stay valid until the schedule changes.
5. **Very large networks.** Precompute the legal connections for each flight when the schedule loads, so a search only follows links.
