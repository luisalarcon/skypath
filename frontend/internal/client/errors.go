package client

import "errors"

// ErrUnavailable is wrapped by every error that means the API could not
// answer: it could not be reached, it failed, or its reply made no sense. It
// says nothing about the request itself, so callers present it as a service
// problem rather than a user mistake.
var ErrUnavailable = errors.New("search service unavailable")
