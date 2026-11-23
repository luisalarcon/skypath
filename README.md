# SkyPath

Flight connection search. Given an origin, a destination and a date, SkyPath finds every valid itinerary of up to two stops, with durations that are correct across time zones, daylight saving and the date line.

Two small Go services with no third-party dependencies:

- **backend**: a versioned JSON API over an in-memory flight index
- **frontend**: a server-rendered web application

## Quick start

```sh
docker compose up --build
```

| Service | URL |
|---|---|
| Web application | http://localhost:3000 |
| API | http://localhost:8080 |

## Documentation

| Document | Contents |
|---|---|
| [Architecture](docs/architecture.md) | Services, request flow, package layout |
| [Data](docs/data.md) | The schedule file, validation, storage and indexes |
| [Search](docs/search.md) | Time model, connection rules and the algorithm |
| [API](docs/api.md) | Endpoints, validation and errors; contract in [`api/openapi.yaml`](api/openapi.yaml) |
| [Frontend](docs/frontend.md) | Screens, states and visual design |
| [Testing](docs/testing.md) | Test strategy and reference results |
| [Operations](docs/operations.md) | Configuration, health, containers, deployment |
| [Decisions](docs/decisions.md) | Why it is built this way |
