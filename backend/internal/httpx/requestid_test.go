package httpx

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestValidRequestID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"3f2a9c1e7b4d4a10", true},
		{"a", true},
		{"Req-1_2.3", true},
		{strings.Repeat("x", 64), true},
		{"", false},
		{strings.Repeat("x", 65), false},
		{"has space", false},
		{"semi;colon", false},
		{"new\nline", false},
		{"slash/inside", false},
		{"quote\"d", false},
		{"café", false},
	}
	for _, tc := range tests {
		if got := ValidRequestID(tc.id); got != tc.want {
			t.Errorf("ValidRequestID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestRequestID(t *testing.T) {
	generated := regexp.MustCompile(`^[0-9a-f]{16}$`)

	tests := []struct {
		name     string
		inbound  string
		wantSame bool // the inbound value must be kept
	}{
		{"well-formed ID is accepted", "3f2a9c1e7b4d4a10", true},
		{"missing ID is generated", "", false},
		{"malformed ID is replaced", "not valid!", false},
		{"overlong ID is replaced", strings.Repeat("a", 65), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen string
			h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = RequestIDFromContext(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.inbound != "" {
				req.Header.Set(RequestIDHeader, tc.inbound)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			echoed := rec.Header().Get(RequestIDHeader)
			if echoed != seen {
				t.Errorf("response header %q differs from context value %q", echoed, seen)
			}
			if tc.wantSame {
				if echoed != tc.inbound {
					t.Errorf("ID = %q, want inbound %q", echoed, tc.inbound)
				}
				return
			}
			if !generated.MatchString(echoed) {
				t.Errorf("generated ID %q is not 16 hex characters", echoed)
			}
		})
	}
}

func TestRequestIDsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		id := newRequestID()
		if seen[id] {
			t.Fatalf("duplicate request ID %q", id)
		}
		seen[id] = true
	}
}

func TestRequestIDFromContextWithoutMiddleware(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := RequestIDFromContext(req.Context()); got != "" {
		t.Errorf("RequestIDFromContext = %q, want empty", got)
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	tag := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+" in")
				next.ServeHTTP(w, r)
				order = append(order, name+" out")
			})
		}
	}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	}), tag("a"), tag("b"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a in", "b in", "handler", "b out", "a out"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestChainWithoutMiddleware(t *testing.T) {
	called := false
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Error("handler was not called")
	}
}
