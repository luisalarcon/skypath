package httpx

import "net/http"

// statusWriter records what a handler wrote so that middleware can log the
// status and size, or tell whether a response has already started.
type statusWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

// WriteHeader records the first final status code. Informational (1xx)
// responses other than 101 may precede the real one, so they pass through
// without counting as the response.
func (w *statusWriter) WriteHeader(code int) {
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write counts the bytes written and, like net/http, treats the first write
// as an implicit 200.
func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer for
// Flush, Hijack and deadlines.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
