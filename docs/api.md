# API

The backend exposes a small, versioned JSON API. The machine-readable contract is [`api/openapi.yaml`](../api/openapi.yaml); the payloads in [`api/examples/`](../api/examples) are canonical and are asserted by tests on both the backend and the frontend.

Base URL when running locally: `http://localhost:8080`.

| Method and path | Purpose |
|---|---|
| `GET /api/v1/search` | Search itineraries |
| `GET /api/v1/airports` | List airports |
| `GET /api/v1/dataset` | Describe the loaded schedule |
| `GET /healthz` | Liveness |
| `GET /readyz` | Readiness and load report |

## Search

```
GET /api/v1/search?origin=JFK&destination=LAX&date=2024-03-15
```

| Parameter | Required | Meaning |
|---|---|---|
| `origin` | yes | IATA code of the departure airport |
| `destination` | yes | IATA code of the arrival airport |
| `date` | yes | Departure date at the origin, `YYYY-MM-DD` |

- Codes are trimmed and upper-cased before use, so `jfk` and ` JFK ` are accepted.
- `date` is the local calendar date of the **first departure at the origin**. Later flights in a connection may depart on a following day.
- Unknown query parameters are ignored. If a parameter is repeated, the first value is used.
- A valid search with no matches returns `200` and an empty `itineraries` list.

**Response**

```json
{
  "origin": {"code": "JFK", "name": "John F. Kennedy International", "city": "New York", "country": "US", "timezone": "America/New_York"},
  "destination": {"code": "LAX", "name": "Los Angeles International", "city": "Los Angeles", "country": "US", "timezone": "America/Los_Angeles"},
  "date": "2024-03-15",
  "currency": "USD",
  "count": 27,
  "itineraries": [
    {
      "stops": 1,
      "departureTime": "2024-03-15T07:00:00-04:00",
      "arrivalTime": "2024-03-15T11:30:00-07:00",
      "totalDurationMinutes": 450,
      "totalPrice": 414.00,
      "segments": [
        {
          "flightNumber": "SP110", "airline": "SkyPath Airways", "aircraft": "A319",
          "origin": {"code": "JFK", "...": "..."},
          "destination": {"code": "ORD", "...": "..."},
          "departureTime": "2024-03-15T07:00:00-04:00",
          "arrivalTime": "2024-03-15T08:30:00-05:00",
          "durationMinutes": 150,
          "price": 189.00
        }
      ],
      "layovers": [
        {"airport": {"code": "ORD", "...": "..."}, "durationMinutes": 45, "type": "domestic"}
      ]
    }
  ]
}
```

- **Times** are RFC 3339 with the airport's UTC offset: the local wall-clock time and an unambiguous instant in one value.
- **Durations** are whole minutes. `totalDurationMinutes` runs from the first departure to the last arrival, layovers included.
- **Prices** are decimal amounts in `currency`, always with two decimals. `totalPrice` is the sum of the segment prices.
- **`layovers[i]`** is the connection between `segments[i]` and `segments[i+1]`. A direct itinerary has an empty list.
- **Order:** total duration, then price, then departure time. The order is deterministic.

## Validation

Every problem with a request is reported in one response, so a client can show all of them at once. Fields are checked independently and reported in the order `origin`, `destination`, `date`.

For each airport field, the first failing check is reported:

| Check | `code` | `message` |
|---|---|---|
| Missing or blank | `required` | `Origin is required.` / `Destination is required.` |
| Not three letters | `invalid_format` | `Origin must be a three-letter airport code.` / `Destination must be a three-letter airport code.` |
| Not in the schedule | `unknown_airport` | `Unknown airport code XXX.` |

For `date`:

| Check | `code` | `message` |
|---|---|---|
| Missing or blank | `required` | `Date is required.` |
| Not a real date in `YYYY-MM-DD` form | `invalid_format` | `Date must be a valid date in YYYY-MM-DD format.` |

When both codes are valid, known airports and are the same:

| Check | `field` | `code` | `message` |
|---|---|---|---|
| Origin equals destination | `destination` | `same_as_origin` | `Destination must be different from origin.` |

Messages are written for end users and are safe to display as they are.

## Errors

Errors use the problem-details format (RFC 9457) with `Content-Type: application/problem+json`.

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "The search request is invalid.",
  "code": "validation_failed",
  "requestId": "3f2a9c1e7b4d4a10",
  "errors": [
    {"field": "origin", "code": "unknown_airport", "message": "Unknown airport code XXX."}
  ]
}
```

| Status | `code` | When | `detail` |
|---|---|---|---|
| 400 | `validation_failed` | A parameter is missing or invalid; `errors` lists each one | `The search request is invalid.` |
| 404 | `not_found` | No such path | `The requested resource was not found.` |
| 405 | `method_not_allowed` | Path exists, method does not; an `Allow` header is set | `The request method is not supported for this resource.` |
| 503 | `timeout` | The request exceeded the server's time limit | `The request took too long to complete. Please try again.` |
| 500 | `internal_error` | Anything unexpected | `The request could not be completed. Please try again.` |

- `type` is `about:blank`, so `title` is the HTTP status phrase, as the RFC specifies. `code` is the stable identifier clients should switch on.
- A 500 never exposes internal details. The cause is written to the log with the same `requestId`.

## Headers

| Header | Behaviour |
|---|---|
| `X-Request-ID` | Taken from the request when it is 1 to 64 characters of letters, digits, `.`, `_` or `-`; otherwise generated. Returned on every response and repeated as `requestId` in problem bodies. |
| `Content-Type` | `application/json; charset=utf-8`, or `application/problem+json; charset=utf-8` for errors |
| `Cache-Control` | `no-store` |
| `X-Content-Type-Options` | `nosniff` |

## Reference data

`GET /api/v1/airports` returns every airport, ordered by code:

```json
{"count": 25, "airports": [{"code": "AMS", "name": "...", "city": "Amsterdam", "country": "NL", "timezone": "Europe/Amsterdam"}]}
```

`GET /api/v1/dataset` describes the loaded schedule:

```json
{"airports": 25, "flights": 302, "firstDepartureDate": "2024-03-15", "lastDepartureDate": "2024-03-16", "skippedRecords": 1}
```

The date range lets a client choose a sensible default date and explain an empty result.

## Health

- `GET /healthz` returns `{"status": "ok"}` whenever the process can serve requests.
- `GET /readyz` returns `200` once the schedule is loaded, with what was loaded and every rejected source record:

```json
{
  "status": "ready",
  "dataset": {
    "airports": 25,
    "flights": 302,
    "skipped": [{"record": "flight SP995", "reason": "unknown origin airport \"JKF\""}]
  }
}
```

The schedule is loaded before the server starts listening, so in practice the service is ready as soon as it accepts connections.

## Examples

```sh
# Direct and connecting itineraries
curl 'http://localhost:8080/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15'

# No direct flight exists; connections only
curl 'http://localhost:8080/api/v1/search?origin=BOS&destination=SEA&date=2024-03-15'

# Crosses the date line
curl 'http://localhost:8080/api/v1/search?origin=SYD&destination=LAX&date=2024-03-15'

# Validation errors
curl -i 'http://localhost:8080/api/v1/search?origin=XXX&destination=LAX&date=2024-03-15'
curl -i 'http://localhost:8080/api/v1/search?origin=JFK&destination=JFK&date=2024-03-15'
```
