package web

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testClock is a clock the tests move by hand.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// scriptedLoad returns a loader that yields the queued results in order and
// counts its calls.
type scriptedLoad struct {
	results []loadResult
	calls   atomic.Int32
}

type loadResult struct {
	value string
	err   error
}

func (s *scriptedLoad) load(context.Context) (string, error) {
	i := int(s.calls.Add(1)) - 1
	if i >= len(s.results) {
		i = len(s.results) - 1
	}
	return s.results[i].value, s.results[i].err
}

func newTestCache(clock *testClock, s *scriptedLoad) *cache[string] {
	c := newCache("test data", time.Minute, 10*time.Second, discardLogger(), s.load)
	c.now = clock.Now
	return c
}

func TestCacheServesFromMemoryUntilExpiry(t *testing.T) {
	clock := newTestClock()
	s := &scriptedLoad{results: []loadResult{{value: "first"}, {value: "second"}}}
	c := newTestCache(clock, s)

	for i := 0; i < 3; i++ {
		if got, ok := c.get(context.Background()); !ok || got != "first" {
			t.Fatalf("get = %q, %v; want first, true", got, ok)
		}
	}
	if n := s.calls.Load(); n != 1 {
		t.Errorf("loads = %d, want 1", n)
	}

	clock.Advance(59 * time.Second)
	if got, _ := c.get(context.Background()); got != "first" {
		t.Errorf("value changed before the ttl: %q", got)
	}

	clock.Advance(2 * time.Second)
	if got, ok := c.get(context.Background()); !ok || got != "second" {
		t.Errorf("get after ttl = %q, %v; want second, true", got, ok)
	}
	if n := s.calls.Load(); n != 2 {
		t.Errorf("loads = %d, want 2", n)
	}
}

func TestCacheKeepsServingStaleValueWhenRefreshFails(t *testing.T) {
	clock := newTestClock()
	s := &scriptedLoad{results: []loadResult{{value: "good"}, {err: errors.New("backend down")}}}
	c := newTestCache(clock, s)
	c.get(context.Background())

	clock.Advance(2 * time.Minute)
	if got, ok := c.get(context.Background()); !ok || got != "good" {
		t.Errorf("get = %q, %v; want the stale value", got, ok)
	}
}

func TestCacheRemembersFailuresForTheRetryInterval(t *testing.T) {
	clock := newTestClock()
	s := &scriptedLoad{results: []loadResult{{err: errors.New("down")}, {value: "recovered"}}}
	c := newTestCache(clock, s)

	if _, ok := c.get(context.Background()); ok {
		t.Fatal("get reported a value from a failed load")
	}
	clock.Advance(9 * time.Second)
	if _, ok := c.get(context.Background()); ok {
		t.Error("get reported a value inside the retry window")
	}
	if n := s.calls.Load(); n != 1 {
		t.Errorf("loads = %d inside the retry window, want 1", n)
	}

	clock.Advance(2 * time.Second)
	if got, ok := c.get(context.Background()); !ok || got != "recovered" {
		t.Errorf("get after retry window = %q, %v; want recovered, true", got, ok)
	}
}

func TestCacheDoesNotPunishTheBackendForACancelledRequest(t *testing.T) {
	clock := newTestClock()
	s := &scriptedLoad{results: []loadResult{{err: context.Canceled}, {value: "fine"}}}
	c := newTestCache(clock, s)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := c.get(ctx); ok {
		t.Fatal("get reported a value from a failed load")
	}

	if got, ok := c.get(context.Background()); !ok || got != "fine" {
		t.Errorf("next visitor got %q, %v; want a fresh load", got, ok)
	}
}

func TestCacheLogsRefreshFailures(t *testing.T) {
	var buf bytes.Buffer
	clock := newTestClock()
	s := &scriptedLoad{results: []loadResult{{err: errors.New("backend down")}}}
	c := newCache("airports", time.Minute, time.Second, slog.New(slog.NewTextHandler(&buf, nil)), s.load)
	c.now = clock.Now

	c.get(context.Background())
	out := buf.String()
	for _, want := range []string{"level=WARN", "data=airports", "backend down"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q does not contain %q", out, want)
		}
	}
}

func TestCacheIsSafeForConcurrentUse(t *testing.T) {
	clock := newTestClock()
	s := &scriptedLoad{results: []loadResult{{value: "shared"}}}
	c := newTestCache(clock, s)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, ok := c.get(context.Background()); !ok || got != "shared" {
				t.Errorf("get = %q, %v", got, ok)
			}
		}()
	}
	wg.Wait()

	if n := s.calls.Load(); n != 1 {
		t.Errorf("concurrent callers caused %d loads, want 1", n)
	}
}
