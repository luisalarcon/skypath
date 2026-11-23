// Package httpx provides small, independent HTTP middleware: request IDs,
// access logging, panic recovery, request deadlines and response headers.
//
// The package knows nothing about the API's wire format. Where a middleware
// has to write an error body it takes a callback, so the API layer decides
// what that body looks like.
package httpx

import "net/http"

// Middleware wraps a handler with additional behaviour.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware to h so that the first one listed is the
// outermost: it sees the request first and the response last.
func Chain(h http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
