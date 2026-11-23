// Package web is the HTTP layer of the frontend: routing, the search page
// handler, static assets, security headers and error pages. It turns API
// responses into view models and hands them to the templates.
//
// Public API:
//
//	NewHandler(deps Dependencies) http.Handler
//
// Routes:
//
//	GET /         the search page; runs a search when origin, destination
//	              and date are present in the query string
//	GET /static/  embedded stylesheet and script
//	GET /healthz  liveness
//	anything else the 404 error page
//
// The response status mirrors the outcome while the body is always a full
// page: 200 for a completed search (including one with no results), 400 when
// the input is invalid, 502 when the search service fails and 504 when it
// times out.
package web
