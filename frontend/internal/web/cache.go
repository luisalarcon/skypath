package web

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/luisalarcon/skypath/frontend/internal/client"
)

// cache holds one value fetched from the API, such as the airport list.
// Reference data changes only when the backend is redeployed, so the page
// reads it from memory and refreshes it at most once per ttl. It is safe for
// concurrent use.
//
// A failed refresh does not make the page worse than it was: the previous
// value, if there is one, keeps being served. The failure is remembered for
// the retry interval so a struggling backend is not asked again by every
// page view, each of which would wait out its timeout.
type cache[T any] struct {
	name   string // for logs
	ttl    time.Duration
	retry  time.Duration
	load   func(context.Context) (T, error)
	logger *slog.Logger
	now    func() time.Time

	// mu is held while loading, so concurrent page views wait for one fetch
	// instead of each making their own.
	mu      sync.Mutex
	value   T
	have    bool
	expires time.Time
}

func newCache[T any](name string, ttl, retry time.Duration, logger *slog.Logger, load func(context.Context) (T, error)) *cache[T] {
	return &cache[T]{name: name, ttl: ttl, retry: retry, load: load, logger: logger, now: time.Now}
}

// get returns the cached value, refreshing it first when it has expired. The
// boolean is false when no value has ever been loaded successfully.
func (c *cache[T]) get(ctx context.Context) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.now().Before(c.expires) {
		return c.value, c.have
	}

	value, err := c.load(ctx)
	if err == nil {
		c.value, c.have = value, true
		c.expires = c.now().Add(c.ttl)
		return c.value, true
	}

	// A caller that gave up says nothing about the backend, so it must not
	// start a retry window that would hide good data from the next visitor.
	if ctx.Err() == nil {
		c.expires = c.now().Add(c.retry)
		c.logger.Warn("reference data unavailable",
			"data", c.name,
			"request_id", client.RequestID(ctx),
			"serving_stale", c.have,
			"error", err)
	}
	return c.value, c.have
}
