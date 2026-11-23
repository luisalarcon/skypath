package api

import (
	"log/slog"
	"net/http"

	"github.com/luisalarcon/skypath/backend/internal/httpx"
)

// allowedMethods is the Allow header for every resource. Every route is read
// only; net/http answers HEAD wherever GET is registered.
const allowedMethods = "GET, HEAD"

// server holds what the handlers share.
type server struct {
	deps   Dependencies
	logger *slog.Logger
}

// route is one resource the API serves.
type route struct {
	path    string
	handler http.HandlerFunc
}

// NewHandler returns the complete handler for the service: every route, with
// the middleware chain applied. Outermost first, the chain is request ID,
// access log, panic recovery, request deadline and response headers. The
// request ID comes first so that every log line, including the access log's,
// carries it; recovery sits inside the access log so that a recovered panic
// is logged with its 500.
func NewHandler(deps Dependencies) http.Handler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &server{deps: deps, logger: logger}

	routes := s.routes()
	known := make(map[string]bool, len(routes))
	mux := http.NewServeMux()
	for _, rt := range routes {
		mux.HandleFunc("GET "+rt.path, rt.handler)
		known[rt.path] = true
	}
	// A pattern without a method matches everything the routes above did not,
	// which lets the API answer with problem documents instead of the
	// router's plain-text 404 and 405.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if known[r.URL.Path] {
			w.Header().Set("Allow", allowedMethods)
			writeProblem(w, r, problemMethod, nil)
			return
		}
		writeProblem(w, r, problemNotFound, nil)
	})

	return httpx.Chain(mux,
		httpx.RequestID(),
		httpx.AccessLog(logger),
		httpx.Recover(logger, respondInternalError),
		httpx.Deadline(deps.RequestTimeout),
		httpx.SecureHeaders(),
	)
}

func (s *server) routes() []route {
	return []route{
		{"/api/v1/airports", s.handleAirports},
		{"/api/v1/dataset", s.handleDataset},
		{"/healthz", s.handleHealth},
		{"/readyz", s.handleReady},
	}
}
