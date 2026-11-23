package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeadlineSetsContextDeadline(t *testing.T) {
	var (
		deadline time.Time
		has      bool
	)
	h := Deadline(time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, has = r.Context().Deadline()
	}))

	before := time.Now()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !has {
		t.Fatal("request context has no deadline")
	}
	if remaining := deadline.Sub(before); remaining <= 0 || remaining > time.Minute {
		t.Errorf("deadline is %s from now, want within (0, 1m]", remaining)
	}
}

func TestDeadlineExpires(t *testing.T) {
	var err error
	h := Deadline(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			err = r.Context().Err()
		case <-time.After(5 * time.Second):
		}
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if err != context.DeadlineExceeded {
		t.Errorf("context error = %v, want context.DeadlineExceeded", err)
	}
}

func TestDeadlineIsReleasedAfterHandlerReturns(t *testing.T) {
	var ctx context.Context
	h := Deadline(time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if ctx.Err() != context.Canceled {
		t.Errorf("context error after return = %v, want context.Canceled (timer released)", ctx.Err())
	}
}

func TestDeadlineKeepsEarlierParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	parentDeadline, _ := parent.Deadline()

	var got time.Time
	h := Deadline(time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = r.Context().Deadline()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent))

	if !got.Equal(parentDeadline) {
		t.Errorf("deadline = %v, want the earlier parent deadline %v", got, parentDeadline)
	}
}

func TestDeadlineZeroMeansNoLimit(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		var has bool
		h := Deadline(d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, has = r.Context().Deadline()
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		if has {
			t.Errorf("Deadline(%s) set a deadline, want none", d)
		}
	}
}
