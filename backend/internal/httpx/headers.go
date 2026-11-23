package httpx

import "net/http"

// SecureHeaders sets the headers every API response carries: Cache-Control:
// no-store, because search results depend on a schedule that may change
// between deploys and must not be served stale from a shared cache, and
// X-Content-Type-Options: nosniff, so browsers honour the declared content
// type.
//
// The headers are set before the handler runs, so error responses written
// further out, such as by Recover, still carry them.
func SecureHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Cache-Control", "no-store")
			h.Set("X-Content-Type-Options", "nosniff")
			next.ServeHTTP(w, r)
		})
	}
}
