# Decision log

Why SkyPath is built the way it is. Each entry records the question, the options considered, what was chosen and what it costs. Entries are added as decisions are made and are not rewritten afterwards; a reversed decision gets a new entry.

## How the project is built

I planned and built SkyPath with Claude Fable 5.2 in Claude Code, working spec-first.

- **Audit, then design.** Before any design work, the schedule file was profiled and an independent prototype of the search was written to produce reference results. The design documents were written around what the data actually contains.
- **Spec before code.** The first plan the assistant produced went straight from requirements to build phases. I stopped that and had the design written down and argued over first. Every entry below was settled before implementation started.
- **Disagreement in both directions.** I overruled the assistant on the language, the frontend approach and the pace. It pushed back on me about the scale of the data and about what a small frontend still has to deliver. Both are recorded below where they happened.
- **Contract first, then parallel.** The domain types, the interfaces between packages, the API contract and the view models were written and frozen first. Five streams then built against them at the same time, each on its own branch and worktree: Claude Opus on the data layer, the search engine and the interface, Claude Sonnet on the two HTTP layers and the infrastructure.
- **Review gates.** Each branch is checked against its specification before it is merged, and the integrated service is checked against the reference results produced during the audit.

---

## 1. Language: Go for both services

**Options.** TypeScript end to end (the assistant's recommendation: one language, shared types); Go.

**Decision.** Go, mine.

**Why it fits.** The standard library covers everything the service needs: an HTTP router with method patterns, the IANA time zone database, structured logging, HTML templating and a test server. Each service compiles to one static binary in a small container image.

**Cost.** No shared types between backend and frontend. The API contract and its example payloads carry that weight instead.

## 2. No third-party dependencies

**Decision.** Standard library only, in both services.

**Why.** Nothing in the problem needs more, and every dependency is something to audit, update and explain. The few things a framework would give for free (middleware chaining, request IDs, problem responses) are small enough to write and test directly.

**Cost.** A little hand-written plumbing, and money and date types that a library might have provided.

## 3. Storage: in-memory indexes, not a database

**Context.** I approached this as a data problem and asked for the design to focus on where the data lives and how it is retrieved.

**Options.** In-memory indexes; embedded SQLite; PostgreSQL as a third service.

**Discussion.** The assistant pushed back on scale: the schedule is 303 flights and 68 KB, read-only, loaded once. A database would add a dependency, a startup-ordering problem and a failure mode in exchange for nothing at this size. I asked whether an in-memory design would still deploy easily; it deploys more easily, because the service has no external dependencies at all.

**Decision.** A read-only in-memory store behind a two-method interface, `FlightSource`.

**Cost.** Changing the schedule means building a new image. Data larger than memory would need a different `FlightSource`; [data.md](data.md) describes that path.

## 4. Retrieval: search per request over a sorted index

**Options.**

1. Index flights by origin, sorted by departure, and search on each request.
2. Precompute every legal connection at startup and follow links at search time.
3. Precompute every itinerary for every route and date.

**Decision.** Option 1. One operation, "flights leaving X at or after T", found by binary search, serves both the first flight and every connection.

**Why.** It is the simplest design that is already fast: a search inspects a few hundred flights. It uses the least memory and keeps the rules evaluated in one place at query time, so they could later vary per search.

**Cost.** The rules are re-evaluated on every request. Option 2 is the natural next step if the network grows.

## 5. Invalid records are skipped and reported, never repaired

**Context.** One flight in the schedule has origin `JKF`, which is not an airport in the file.

**Options.** Correct it to `JFK`; reject the whole file; skip the record.

**Decision.** Skip the record, log it, and list it on the readiness endpoint with its reason.

**Why.** A guessed correction invents a flight that may not exist and hides the problem from whoever owns the data. Rejecting the whole file over one record takes the service down for a typo. A reported skip keeps the service correct and makes the problem visible.

**Cost.** Searches return one fewer JFK to LAX flight than a hand-corrected file would.

## 6. Prices as integer cents; mixed input accepted

**Context.** Prices in the schedule appear as decimals, integers and strings.

**Decision.** Accept a JSON number or a numeric string, parse the text straight to whole cents, and do all arithmetic on integers.

**Why.** Floating point cannot represent most decimal fractions, so sums drift. The inconsistency in the source is cosmetic and safe to normalise, unlike the airport code in entry 5.

## 7. Times are absolute instants that carry their airport's zone

**Decision.** Convert every local time to an instant once, at load, using the IANA zone named for its airport. Compile the zone database into the binary.

**Why.** On the schedule's date the United States is on daylight time, Europe is not, and Phoenix never is. Any fixed-offset shortcut gets at least one of those wrong. With instants, every duration is a subtraction and the date line needs no special case.

**Cost.** Results follow the zone database even where they look odd: the schedule's Phoenix to Los Angeles flight is 30 minutes.

## 8. Reading the connection rules

Three points the rules leave open, and how they are read.

| Question | Reading | Why |
|---|---|---|
| Is a layover of exactly 45, 90 or 360 minutes allowed? | Yes, bounds are inclusive | "Minimum 45" and "maximum 6 hours" name allowed values |
| A domestic flight connecting to an international one? | International, 90 minutes | A connection is domestic only if both flights are within the same country |
| Which date does the search date refer to? | The first departure, in the origin's local time | It is the date the traveller chooses to leave |

The schedule has 46 flight pairs exactly 45 minutes apart, 103 at 90 and 35 at 360, so each of these readings changes results.

## 9. Identical origin and destination is a validation error

**Options.** Return an empty list; return a 400.

**Decision.** 400 with a message on the destination field.

**Why.** It is a mistake in the request, not a search with no answer. An explicit message is more useful than an empty page.

## 10. API shape: versioned paths, an OpenAPI contract, problem-details errors

**Decision.** Paths under `/api/v1`. The contract lives in `api/openapi.yaml` with canonical example payloads that both services test against. Errors use RFC 9457 problem details with a stable `code` and a per-field `errors` list.

**Why.** The contract is what allowed the backend and frontend to be built at the same time. A standard error format means clients do not have to learn a bespoke one, and reporting every invalid field at once lets a form show all its problems together.

## 11. Frontend: server-rendered Go, enhanced with a small script

**Options.** A React single-page application (the assistant's first proposal); static HTML with vanilla JavaScript; Go templates.

**Decision.** A second Go service rendering HTML on the server, mine. The frontend is the smaller part of this product, so it should be the simpler part of the system.

**Discussion.** The assistant pointed out that a small frontend still has to deliver validation, loading, empty and error states properly. I then asked for the interface to look modern and polished rather than merely functional, which led to the design direction in [frontend.md](frontend.md).

**How it works.** Every state renders on the server and works without JavaScript. With script, the form submits in the background and the results are swapped in from the same full-page response, so there is one rendering path.

**Cost.** The loading state needs script. Two Go modules repeat a small amount of middleware rather than share a package.

## 12. The browser talks only to the frontend

**Decision.** The frontend service calls the API server to server. The browser never does.

**Why.** No CORS configuration, no API details in client code, and the page works with script disabled.

**Cost.** One extra hop per search, on a local network.

## 13. Local times that do not exist are rejected

**Context.** Raised during the build of the loader, not in the original design. A local time inside a daylight-saving gap, such as 02:30 on the night clocks go forward, never happens in that zone. Go's parser accepts it and silently moves it to a nearby instant.

**Decision.** The loader rejects the record, with a reason naming the zone.

**Why.** Moving the time is a guess, and entry 5 already settled that the loader does not guess. No record in the current schedule is affected.

## 14. A client that disconnects is not a server error

**Context.** Found while reviewing the API. A search cancelled because the client went away was answered with a 500 and logged as an error.

**Decision.** It is logged at info and recorded as status 499 in the access log. A deadline that expires is different: that is the server's own limit, and still answers 503.

**Why.** Error-level logs should mean something failed on the server. This did not.

## 15. Templates decide nothing

**Context.** The first version of the interface showed "Itineraries 27 · Direct 3", because a template cannot pluralise without logic, and always offered to repeat an empty search on the first covered date, even when that was the date just searched.

**Decision.** The handler supplies the wording and decides whether there is a link to offer. Templates loop and show what they are given.

**Why.** Logic in Go is unit-tested; logic in a template is not.

## 16. Development process: public from the first commit, atomic history

**Decision.** The repository is public from its first push. Commits are atomic, each building and passing on its own, and history is never squashed. Work lands through branches merged with merge commits.

**Why.** The history is part of the documentation: it shows the order in which the system was built and why each step was taken.

## 17. A live demo on a free plan

**Context.** Local Docker is the supported way to run SkyPath. A hosted copy lets someone try it without cloning anything.

**Decision.** Deploy both containers, unchanged, to Render's free plan, built straight from the public repository. The only setting that differs from local is the frontend's `BACKEND_TIMEOUT`, raised to 60 seconds.

**Why.** It shows the images run on a real host with nothing but environment variables, at no cost. The longer timeout exists because free services sleep when idle: without it, the first search after a quiet spell would fail while the API wakes, instead of simply being slow.

**Cost.** The first request after about 15 idle minutes takes up to a minute. The API has its own public URL; on a host with private networking it would be reachable only from the frontend.
