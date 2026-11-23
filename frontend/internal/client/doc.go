// Package client is a small typed client for the flight search API. It owns
// everything about talking to the backend: URLs, timeouts, decoding, and
// turning failures into errors the web layer can present.
//
// The wire format is defined by api/openapi.yaml at the repository root.
//
// Public API:
//
//	New(baseURL string, httpClient *http.Client) *Client
//	(*Client).Search(ctx context.Context, origin, destination, date string) (*SearchResponse, error)
//	(*Client).Airports(ctx context.Context) ([]Airport, error)
//	(*Client).Dataset(ctx context.Context) (*Dataset, error)
//
// Errors:
//
//   - *ProblemError for a 4xx problem-details response. It carries the status,
//     title, detail and the per-field messages, so validation errors can be
//     shown beside the matching input.
//   - An error wrapping ErrUnavailable when the backend cannot be reached,
//     times out, returns a 5xx or returns a body that cannot be decoded.
package client
