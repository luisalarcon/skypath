package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/view"
	"github.com/luisalarcon/skypath/frontend/static"
	"github.com/luisalarcon/skypath/frontend/templates"
)

// Dependencies are the collaborators of the web handler.
type Dependencies struct {
	// API answers searches and supplies reference data. Required.
	API API

	// Logger receives the access log and diagnostics. When nil, nothing is
	// logged.
	Logger *slog.Logger

	// RequestTimeout bounds the handling of one request, including its calls
	// to the API. Zero means no bound beyond the server's own timeouts.
	RequestTimeout time.Duration

	// Render writes a named template. It defaults to templates.Render and
	// exists so tests can observe what a page was given and make rendering
	// fail.
	Render func(w io.Writer, name string, data any) error
}

// handler is the HTTP side of the frontend: it owns the response, while the
// server decides what a page contains.
type handler struct {
	*server
	render         func(w io.Writer, name string, data any) error
	requestTimeout time.Duration
}

// NewHandler returns the frontend's HTTP handler: the search page, static
// assets, a liveness probe and error pages, wrapped in the middleware that
// assigns request IDs, logs, recovers from panics and sets security headers.
//
// It panics if deps.API is nil, since that is a wiring mistake that should
// stop the process at startup rather than fail every request.
func NewHandler(deps Dependencies) http.Handler {
	if deps.API == nil {
		panic("web: Dependencies.API is required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	render := deps.Render
	if render == nil {
		render = templates.Render
	}

	assets := static.FS()
	version, err := assetVersion(assets)
	if err != nil {
		// Without a version, assets are still served but are never marked as
		// cacheable for a year.
		logger.Error("compute asset version", "error", err)
		version = ""
	}

	h := &handler{
		server:         newServer(deps.API, logger, version),
		render:         render,
		requestTimeout: deps.RequestTimeout,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.searchPage)
	mux.Handle("GET "+staticPrefix, staticHandler(assets, version, http.HandlerFunc(h.notFound)))
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("/", h.notFound)

	// The outermost layer comes first: every later one can use the request ID,
	// the access log sees the status the recovery layer produces, and the
	// security headers are already set when a panic is turned into a 500.
	var root http.Handler = mux
	root = securityHeaders(root)
	root = recoverPanics(logger, http.HandlerFunc(h.serverError), root)
	root = accessLog(logger, root)
	return requestID(root)
}

// searchPage serves "/": the search form, with results when the query asks
// for a search.
func (h *handler) searchPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.requestTimeout)
		defer cancel()
	}

	page, status := h.buildPage(ctx, r.URL.Query())
	if err := h.writeHTML(w, status, templates.Search, page); err != nil {
		h.logger.Error("render search page", "request_id", client.RequestID(r.Context()), "error", err)
		h.serverError(w, r)
	}
}

// health reports that the process is alive. It does not call the API: a
// liveness probe that failed whenever the backend did would get the frontend
// restarted for a problem a restart cannot fix.
func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, `{"status":"ok"}`+"\n")
}

// notFound serves the 404 page.
func (h *handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.errorPage(w, r, http.StatusNotFound, "Page not found",
		"We couldn't find that page. Check the address, or go back to the search.")
}

// serverError serves the 500 page. It is used for a failed render and, by the
// recovery middleware, for a panic.
func (h *handler) serverError(w http.ResponseWriter, r *http.Request) {
	h.errorPage(w, r, http.StatusInternalServerError, "Something went wrong",
		"Something went wrong on our side. Please try again in a moment.")
}

// errorPage renders the standalone error page. If even that cannot be
// rendered it falls back to a bare text response with the same status, so the
// visitor always gets an answer that matches the logs.
func (h *handler) errorPage(w http.ResponseWriter, r *http.Request, status int, heading, message string) {
	page := view.ErrorPage{
		Title:        heading + " · SkyPath",
		AssetVersion: h.assetVersion,
		Status:       status,
		Heading:      heading,
		Message:      message,
	}
	if err := h.writeHTML(w, status, templates.Error, page); err != nil {
		h.logger.Error("render error page",
			"request_id", client.RequestID(r.Context()), "status", status, "error", err)
		http.Error(w, http.StatusText(status), status)
	}
}

// writeHTML renders a template into a buffer and only then writes the
// response. A template that fails half way therefore produces an error the
// caller can handle, never a truncated page with a 200 status. Nothing is
// written to w when an error is returned.
func (h *handler) writeHTML(w http.ResponseWriter, status int, name string, data any) error {
	var body bytes.Buffer
	if err := h.render(&body, name, data); err != nil {
		return err
	}

	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	// Pages depend on the live search service and are cheap to rebuild, so
	// browsers must not reuse one without asking.
	header.Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	_, _ = body.WriteTo(w)
	return nil
}
