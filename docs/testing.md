# Testing

## Approach

- **Standard library only.** Tests use `testing`, `net/http/httptest` and nothing else.
- **Table-driven** where a rule has boundaries, so each boundary is one visible row.
- **Each layer alone.** Interfaces between packages mean the search runs against a handful of hand-made flights, the API against a stub search, and the frontend against a fake backend.
- **The real schedule too.** A small set of tests runs the whole backend against `flights.json` and checks the reference results below.
- **Hermetic time zones.** Any test that resolves a time zone imports `time/tzdata`, so results do not depend on what the host has installed.
- **Tests travel with code.** A change and its tests land in the same commit.

## Running

```sh
# in backend/ and in frontend/
go test ./...
go test -race -cover ./...
go vet ./...
gofmt -l .        # prints nothing when formatting is clean
```

`make test`, `make lint` and `make smoke` wrap these for both modules.

## What each package proves

### Backend

| Package | Covers |
|---|---|
| `flight` | Money parsing and formatting, exact sums, date parsing and arithmetic, durations across zones and the date line, itinerary totals |
| `dataset` | Number, integer and string prices; every skip reason; file-level failures; the real file |
| `store` | Lookup hit and miss; departure order; the `at or after` boundary; unknown airport; results cannot corrupt stored data |
| `search` | Every connection rule at its boundary; direct, one-stop and two-stop paths; never three stops; no revisits; overnight connections; origin-local date; ordering and tie-breaks; invalid queries; cancellation |
| `api` | Status codes; the error format; input normalisation; every example in `api/examples` |
| `httpx` | Request ID propagation; panic recovery; timeout; access log fields |
| `config` | Defaults; overrides; rejection of invalid values |

### Frontend

| Package | Covers |
|---|---|
| `client` | Success, validation problem, server error, timeout, malformed body, unreachable backend |
| `web` | Each page state; status codes; view-model mapping; formatting helpers; security headers; 404 |
| `templates` | Each state renders with fixture data; user input is escaped |

## Boundary cases

| Rule | Cases |
|---|---|
| Domestic minimum | 44 minutes rejected; 45 and 46 accepted |
| International minimum | 89 rejected; 90 and 91 accepted |
| Maximum | 360 accepted; 361 rejected |
| Mixed connection | domestic then international at 60 minutes rejected; international then domestic at 60 minutes rejected |
| Airport change | arriving LGA, departing JFK: never a connection |
| Time zones | JFK → LAX is 375 minutes; SYD → LAX is 900; PHX → LAX is 30 |
| Search date | a flight departing 23:30 local is on that date even if it is already tomorrow in UTC |

## Reference results

Expected results for the current `flights.json`, on 2024-03-15. They were produced by an independent implementation of the rules in [search.md](search.md), written separately from the service, and are asserted by the backend's dataset-level tests.

| Search | Itineraries | Direct | One stop | Two stops | Fastest |
|---|---|---|---|---|---|
| JFK → LAX | 27 | 3 | 14 | 10 | 375 min (direct) |
| SFO → NRT | 6 | 2 | 1 | 3 | 720 min (direct) |
| BOS → SEA | 13 | 0 | 4 | 9 | 630 min (via DEN) |
| SYD → LAX | 2 | 2 | 0 | 0 | 900 min (direct) |

Spot checks within those results:

| Search | Itinerary | Duration | Price |
|---|---|---|---|
| JFK → LAX | SP110 JFK→ORD, 45-minute domestic layover, SP120 ORD→LAX | 450 min | 414.00 |
| SFO → NRT | SP998 SFO→LAX, 90-minute international layover, SP500 LAX→NRT | 900 min | 944.00 |
| SFO → NRT | SP981 SFO→DEN, SP212 DEN→LAX, SP501 LAX→NRT | 1260 min | 1137.00 |
| BOS → SEA | SP260 BOS→DEN, SP221 DEN→SEA | 630 min | 428.00 |

Invalid input:

| Search | Result |
|---|---|
| JFK → JFK | 400: origin and destination must be different |
| XXX → LAX | 400: unknown airport code XXX |
| JFK → LAX on 2024-03-20 | 200 with an empty list |

Loading the current file yields 25 airports, 302 flights and 1 skipped record (SP995).

## End to end

```sh
docker compose up --build
curl 'http://localhost:8080/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15'
```

Then, in a browser at `http://localhost:3000`:

1. Run each reference search.
2. Submit an empty form and an unknown code; check the messages appear beside the fields.
3. Stop the backend container and search again; check the error banner appears and the form keeps its values.
4. Repeat one search with JavaScript disabled, at phone width, and in dark mode.
