# Operations

## Running

```sh
docker compose up --build
```

| Service | URL |
|---|---|
| Web application | http://localhost:3000 |
| API | http://localhost:8080 |

If either port is taken, override it:

```sh
FRONTEND_PORT=3001 BACKEND_PORT=8081 docker compose up --build
```

Without Docker, with Go 1.26 or newer:

```sh
# terminal 1
cd backend && FLIGHTS_PATH=../flights.json go run ./cmd/server

# terminal 2
cd frontend && BACKEND_URL=http://localhost:8080 go run ./cmd/web
```

## Configuration

Both services are configured only through environment variables. Each value is validated at startup; an invalid value stops the process with a message naming the variable.

### Backend

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | Port to listen on |
| `FLIGHTS_PATH` | `flights.json` | Path of the schedule file |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `REQUEST_TIMEOUT` | `10s` | Time limit for handling one request |
| `SHUTDOWN_TIMEOUT` | `10s` | Time allowed for in-flight requests on shutdown |

### Frontend

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `3000` | Port to listen on |
| `BACKEND_URL` | `http://localhost:8080` | Base URL of the API |
| `BACKEND_TIMEOUT` | `5s` | Time limit for one call to the API |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `REQUEST_TIMEOUT` | `10s` | Time limit for handling one request |
| `SHUTDOWN_TIMEOUT` | `10s` | Time allowed for in-flight requests on shutdown |

Durations use Go syntax: `500ms`, `5s`, `1m`.

## Startup and shutdown

**Backend startup**

1. Read and validate configuration.
2. Load and validate the schedule. An unusable file stops the process here, before it accepts traffic.
3. Log the load summary: airports, flights, and each skipped record with its reason.
4. Start listening.

Because the data is loaded before the listener opens, the service never serves a search from a partial data set.

**Shutdown.** On `SIGTERM` or `SIGINT` each service stops accepting new connections, lets in-flight requests finish for up to `SHUTDOWN_TIMEOUT`, then exits. `docker compose down` therefore does not cut off a request in progress.

## Health

| Endpoint | Service | Meaning |
|---|---|---|
| `GET /healthz` | both | The process is alive |
| `GET /readyz` | backend | The schedule is loaded; reports counts and skipped records |

Each binary also accepts a `-healthcheck` flag that calls its own `/healthz` and exits `0` or `1`. Container health checks use it, so the images need no shell tools.

## Logging

Structured JSON, one object per line, on standard output.

Every request is assigned an ID. It is accepted from the `X-Request-ID` header when present and well-formed, otherwise generated. The frontend forwards its ID to the backend, so a single user action can be followed across both services.

Access log fields:

| Field | Example |
|---|---|
| `time` | `2024-03-15T12:00:00.123Z` |
| `level` | `INFO` |
| `msg` | `request` |
| `request_id` | `3f2a9c1e7b4d4a10` |
| `method` | `GET` |
| `path` | `/api/v1/search` |
| `status` | `200` |
| `duration_ms` | `0.42` |
| `bytes` | `18342` |

Health probes are logged at `debug`, so they do not drown out real traffic at the default level. Server errors are logged at `error` with the cause; the response body never contains it.

## Limits and timeouts

| Setting | Value |
|---|---|
| Read header timeout | 5s |
| Read timeout | 10s |
| Write timeout | 15s |
| Idle timeout | 60s |
| Request handling | `REQUEST_TIMEOUT` |
| Frontend call to API | `BACKEND_TIMEOUT` |

A panic inside a request is recovered, logged with its stack and answered with a 500. It does not take the process down.

## Containers

- Multi-stage builds produce static binaries; the runtime images are small Alpine images running as an unprivileged user.
- The schedule file is copied into the backend image, so a fresh clone needs nothing from the host besides Docker.
- The time zone database is compiled into the backend binary, so results do not depend on the image's system files.
- The frontend's templates, stylesheet and script are embedded in its binary.
- The frontend starts only after the backend reports healthy.

To run a different schedule, replace `flights.json` and rebuild:

```sh
docker compose up --build
```

## Deployment

Each service is a single stateless container, so any container host works.

- **No external dependencies.** The backend needs no database, cache or volume.
- **Horizontal scaling.** Every backend instance holds an identical read-only copy of the schedule, so instances need no coordination and can sit behind any load balancer.
- **Fast cold start.** Startup is dominated by parsing a 68 KB file, which suits hosts that stop idle services.
- **Configuration.** Set `PORT` if the host assigns one, and point the frontend's `BACKEND_URL` at the backend.

Updating the schedule means building and deploying a new backend image. That is the cost of having no database; see [data.md](data.md) for how the design would change if the schedule had to change at run time.
