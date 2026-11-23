package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxBodyBytes caps how much of a response the client will read. The largest
// legitimate reply is a few dozen kilobytes; the cap only protects the
// frontend's memory from a misbehaving or misdirected backend.
const maxBodyBytes = 4 << 20

const (
	problemMediaType = "application/problem+json"
	codeTimeout      = "timeout"
)

// defaultTimeout applies when New is given no HTTP client.
const defaultTimeout = 5 * time.Second

// Client calls the flight search API. It is safe for concurrent use.
type Client struct {
	baseURL  string
	http     *http.Client
	maxBytes int64
}

// New returns a client for the API at baseURL, for example
// "http://localhost:8080". A nil httpClient gets a client with a five second
// timeout. The caller owns the timeout policy when it supplies its own.
func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		http:     httpClient,
		maxBytes: maxBodyBytes,
	}
}

// Search finds itineraries from origin to destination on date (YYYY-MM-DD).
// The values are sent as given: the API trims, upper-cases and validates
// them, and reports every problem at once.
func (c *Client) Search(ctx context.Context, origin, destination, date string) (*SearchResponse, error) {
	query := url.Values{}
	query.Set("origin", origin)
	query.Set("destination", destination)
	query.Set("date", date)

	var out SearchResponse
	if err := c.get(ctx, "/api/v1/search", query, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Airports lists every airport in the schedule, ordered by code.
func (c *Client) Airports(ctx context.Context) ([]Airport, error) {
	var out airportsResponse
	if err := c.get(ctx, "/api/v1/airports", nil, &out); err != nil {
		return nil, err
	}
	return out.Airports, nil
}

// Dataset describes the loaded schedule, including the dates it covers.
func (c *Client) Dataset(ctx context.Context) (*Dataset, error) {
	var out Dataset
	if err := c.get(ctx, "/api/v1/dataset", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// get performs a GET and decodes a successful JSON reply into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %w", ErrUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")
	if id := RequestID(ctx); id != "" {
		req.Header.Set("X-Request-ID", id)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return unavailable(err)
	}
	defer resp.Body.Close()

	body, err := c.readBody(resp.Body)
	if err != nil {
		return unavailable(fmt.Errorf("read response: %w", err))
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		if err := json.Unmarshal(body, out); err != nil {
			return unavailable(fmt.Errorf("decode response: %w", err))
		}
		return nil
	case resp.StatusCode >= 400 && resp.StatusCode <= 499 && isProblem(resp):
		var problem ProblemError
		if err := json.Unmarshal(body, &problem); err != nil {
			return unavailable(fmt.Errorf("decode problem response: %w", err))
		}
		return &problem
	default:
		return statusError(resp.StatusCode, body)
	}
}

// isProblem reports whether the response is a problem-details document.
func isProblem(resp *http.Response) bool {
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return err == nil && mediaType == problemMediaType
}

// statusError describes a response that is neither a success nor a problem
// the caller can act on. It reads the problem body when there is one, only
// to make the message useful when correlating logs; failure to decode it is
// not worth reporting.
func statusError(status int, body []byte) error {
	var problem ProblemError
	_ = json.Unmarshal(body, &problem)

	detail := fmt.Sprintf("unexpected status %d", status)
	if problem.Code != "" {
		detail += fmt.Sprintf(" (%s, request %s)", problem.Code, problem.RequestID)
	}

	// The API reports its own time limit as 503 "timeout"; a proxy in front
	// of it reports one as 504. Both mean the search took too long, which
	// the page words differently from a failure.
	if status == http.StatusGatewayTimeout || problem.Code == codeTimeout {
		return unavailable(fmt.Errorf("%w: %s", ErrTimeout, detail))
	}
	return unavailable(errors.New(detail))
}

// readBody reads the whole body, failing rather than truncating when it is
// longer than the cap: a truncated JSON document would only fail later with
// a more confusing message.
func (c *Client) readBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, c.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > c.maxBytes {
		return nil, fmt.Errorf("body exceeds %d bytes", c.maxBytes)
	}
	return body, nil
}
