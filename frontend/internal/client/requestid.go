package client

import "context"

type requestIDKey struct{}

// WithRequestID returns a context carrying the ID of the request being
// served. Calls made with it send the ID as X-Request-ID, so one user action
// can be followed through the logs of both services.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the ID stored by WithRequestID, or "" when there is none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
