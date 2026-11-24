# Frontend

The frontend is a server-rendered web application: one page that searches and shows itineraries. It is a Go service using `html/template`, a hand-written stylesheet and a small script. There is no JavaScript framework and no build step.

## Principles

1. **The server renders everything.** The page is complete and usable with JavaScript turned off. Script only improves it.
2. **The URL is the state.** A search is a `GET` with three query parameters, so every result can be bookmarked, shared and reloaded.
3. **Be honest about time.** Every time is shown in its own airport's local time, and the interface says so. Arrivals on a different calendar day are marked.
4. **Say what went wrong, next to where it went wrong.** Field problems appear beside the field; service problems appear as a banner; the form never loses what the user typed.
5. **Accessible by construction.** Real labels, real buttons, keyboard-first, announced updates.

## Routes

| Route | Response |
|---|---|
| `GET /` | The search page. Runs a search when `origin`, `destination` or `date` is present in the query string. |
| `GET /static/…` | Embedded stylesheet and script |
| `GET /healthz` | Liveness |
| anything else | The 404 page |

The body is always a complete page. The status reflects the outcome:

| Outcome | Status |
|---|---|
| Page with no search, or a completed search (including no results) | 200 |
| The search input is invalid | 400 |
| The search service failed or is unreachable | 502 |
| The search service timed out | 504 |
| Unknown path | 404 |
| Unexpected failure in the frontend | 500 |

## Page anatomy

```
┌──────────────────────────────────────────────────────────┐
│ SkyPath                                                  │  header
├──────────────────────────────────────────────────────────┤
│ ┌──────────────────────────────────────────────────────┐ │
│ │ From [JFK]  ⇄  To [LAX]   Date [2024-03-15] [Search] │ │  search panel
│ │ New York · JFK     Los Angeles · LAX                 │ │
│ └──────────────────────────────────────────────────────┘ │
│                                                          │
│ (notice banner, when the search service is unavailable)  │
│                                                          │
│ New York (JFK) → Los Angeles (LAX)                       │  results heading
│ Friday, March 15, 2024 · 27 itineraries                  │
│ 3 direct · 14 one stop · 10 two stops · times are local  │
│                                                          │
│ ┌──────────────────────────────────────────────────────┐ │
│ │ 7:00 AM → 11:30 AM     7h 30m · 1 stop ORD   $414.00 │ │  itinerary card
│ │ ● 7:00 AM  JFK New York            SP110 · A319      │ │
│ │ │ 2h 30m                                             │ │
│ │ ● 8:30 AM  ORD Chicago                               │ │
│ │ ┆ 45m layover in Chicago (ORD) · domestic            │ │
│ │ ● 9:15 AM  ORD Chicago             SP120 · A320      │ │
│ │ │ 4h 15m                                             │ │
│ │ ● 11:30 AM LAX Los Angeles                           │ │
│ └──────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

## Search form

| Field | Control | Notes |
|---|---|---|
| From | Text input, three letters | Suggestions from the airport list; upper-cased as typed |
| To | Text input, three letters | Same |
| Date | Native date picker | Defaults to the first date the schedule covers; bounded by the schedule's date range |
| Search | Submit button | |

- Each airport input offers suggestions through a `<datalist>` listing every airport as code, city and name.
- Under each airport input, a hint shows the airport the code resolves to, for example "New York · John F. Kennedy International".
- A swap control exchanges From and To. It needs script, so it is hidden when script is unavailable.
- The form submits with `GET` to `/`.

### Validation

Validation happens in two places, with identical wording.

- **In the browser**, before the request: required fields, three-letter codes, a code that is not in the suggestion list, identical origin and destination. Messages appear under the field as the user leaves it and on submit.
- **On the server**, always: the API is the authority. Its per-field messages are shown under the matching field. See the validation table in [api.md](api.md).

An invalid field is marked with `aria-invalid`, its message is linked with `aria-describedby`, and focus moves to the first invalid field on submit.

## Results

**Heading.** Route with city names and codes, the long date, a summary in words ("27 itineraries · 3 direct · 14 one stop · 10 two stops"), and a note that results are sorted by total travel time and that all times are local to each airport. The handler words the summary, so templates never pluralise.

**Itinerary card.** One per itinerary, fastest first.

- **Summary row:** departure and arrival times, total duration, a stops label with connecting airport codes, and the total price. The fastest and cheapest itineraries carry a small tag.
- **Timeline:** every flight in order, always visible. Each flight shows its departure time, airport code and city; flight number, airline and aircraft; its duration; and its arrival time, airport code and city.
- **Layovers:** between flights, the wait, the airport and whether the connection is domestic or international.
- **Day markers:** an arrival on a different calendar date from its departure shows "+1 day" (or "+2 days", "-1 day") beside the time.

Times use the `<time>` element with the full timestamp in `datetime`.

## States

| State | What the user sees |
|---|---|
| **Initial** | The form, with a short prompt and the dates the schedule covers |
| **Loading** | The button shows a spinner and "Searching…"; skeleton cards fill the results area; the area is marked busy |
| **Results** | Heading and cards |
| **Empty** | "No itineraries found", naming the route and date and the dates the schedule covers. When the search was for a different date, a link repeats it on the first covered date. |
| **Invalid input** | Messages beside the fields; no results area |
| **Service unavailable** | A banner: the search is temporarily unavailable, the search is still filled in, try again. The form keeps its values. |
| **Not found / error page** | A simple page with a link back to search |

The loading state needs script. Without it, the browser's own navigation indicator is the feedback.

## Progressive enhancement

When script is available:

1. Submitting the form is intercepted. Browser-side validation runs first.
2. The loading state is shown and the same URL the form would have navigated to is fetched.
3. The response is a complete page. The script takes the results area, the notice area and the field messages from it and swaps them into the current page.
4. The address bar is updated with `history.pushState`; back and forward restore earlier searches.
5. Focus moves to the results heading, and a polite live region announces the outcome ("27 itineraries found").
6. A new search aborts one still in flight.
7. If the request fails outright, the script falls back to a normal navigation so the server can render the error.

Because the fetched response is the ordinary page, there is one rendering path and no separate fragment endpoint to keep in step.

## Formatting

All formatting happens in Go before values reach a template.

| Value | Format | Example |
|---|---|---|
| Time | 12-hour with AM/PM | `7:00 AM` |
| Short date | weekday, month, day | `Fri, Mar 15` |
| Long date | full weekday and month | `Friday, March 15, 2024` |
| Schedule date | month, day, year | `Mar 15, 2024` |
| Duration | hours and minutes | `45m`, `2h`, `7h 30m` |
| Price | dollars with separators | `$414.00`, `$1,137.00` |
| UTC offset | hours, minutes only if needed | `UTC-4`, `UTC+9`, `UTC+5:30`, `UTC` |
| Day marker | arrival date relative to departure date | `+1 day`, `+2 days`, `-1 day`, or nothing |
| Stops | | `Direct`, `1 stop`, `2 stops` |
| Layover | | `Domestic connection`, `International connection` |

Page titles: `SkyPath · Flight search` before a search and `JFK to LAX · Mar 15, 2024 · SkyPath` with results.

## Visual design

**Character.** Calm, precise and modern: generous white space, strong typographic hierarchy, restrained colour. It should feel like a tool built by people who care about detail, not like a template.

**Layout**

- A single centred column, at most about 56rem wide.
- The search panel is the most prominent element. Its fields sit in one row on wide screens and stack on narrow ones.
- Cards are separated by space, not heavy borders.
- Fully usable from 320px wide to large desktops, with no horizontal scrolling.

**Typography**

- System font stack. No web fonts are downloaded.
- Times and prices use tabular numerals so columns align.
- Clear hierarchy: times and prices are the largest things on a card; metadata such as flight numbers and aircraft is small and muted.

**Colour**

- A neutral palette with one accent colour, used for the primary button, focus rings, links and the timeline.
- Semantic colours for error and warning, never used as the only signal.
- Light and dark themes, following the system setting. Both meet WCAG AA contrast for text.
- All colours, spacing, radii and shadows are CSS custom properties defined once.

**Motion**

- Short, subtle transitions on hover, focus and when results appear.
- Skeleton cards shimmer while loading.
- All motion is disabled under `prefers-reduced-motion`.

## Accessibility

- Semantic landmarks: `header`, `main`, `footer`. One `h1`.
- Every input has a visible `label`. Hints and errors are linked with `aria-describedby`.
- Everything works with the keyboard alone; focus is always clearly visible.
- A separate polite status region announces each outcome ("27 itineraries found"). The results area itself is only marked busy while loading, so replacing the list does not read every card aloud.
- Itineraries are a list. Each card has an accessible name summarising it.
- Colour is never the only carrier of meaning: tags and connection types are text.
- Touch targets are at least 44 by 44 CSS pixels.
- Text can be zoomed to 200% without loss of content.

## Security

- User input is only ever rendered through `html/template`, which escapes by context.
- Responses carry a strict content security policy: scripts, styles and images from the same origin only, no inline script or style, no framing, forms may only submit to the same origin.
- `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin` and `X-Frame-Options: DENY` are set.
- The frontend never forwards arbitrary input to the API beyond the three search values, which the API validates.

## Assets

The stylesheet and script are embedded in the binary and served from `/static/`. Their URLs carry a version derived from their content, so they can be cached for a long time and a new build is picked up immediately. Nothing is loaded from another origin, so the application works offline.
