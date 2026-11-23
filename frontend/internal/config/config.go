// Package config reads the frontend's settings from environment variables.
// Every value is validated once at startup so a typo stops the process with a
// message naming the variable, instead of surfacing later as odd behaviour.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Names of the environment variables the frontend reads.
const (
	EnvPort            = "PORT"
	EnvBackendURL      = "BACKEND_URL"
	EnvBackendTimeout  = "BACKEND_TIMEOUT"
	EnvLogLevel        = "LOG_LEVEL"
	EnvLogFormat       = "LOG_FORMAT"
	EnvRequestTimeout  = "REQUEST_TIMEOUT"
	EnvShutdownTimeout = "SHUTDOWN_TIMEOUT"
)

// Log formats accepted by LOG_FORMAT.
const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

// Config holds the validated frontend settings.
type Config struct {
	// Port is the TCP port to listen on.
	Port int
	// BackendURL is the API's base URL, without a trailing slash.
	BackendURL string
	// BackendTimeout limits one call to the API.
	BackendTimeout time.Duration
	// LogLevel is the minimum level that is logged.
	LogLevel slog.Level
	// LogFormat is LogFormatJSON or LogFormatText.
	LogFormat string
	// RequestTimeout limits the handling of one request.
	RequestTimeout time.Duration
	// ShutdownTimeout is how long in-flight requests may finish on shutdown.
	ShutdownTimeout time.Duration
}

// Addr returns the listen address for net/http, for example ":3000".
func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(os.Getenv)
}

// LoadFrom reads the configuration through getenv, which lets tests supply
// values without touching the real environment. A variable that is unset or
// blank takes its default, because container tooling often passes empty
// values for settings the operator did not choose.
//
// All invalid values are reported together, each prefixed with its variable.
func LoadFrom(getenv func(string) string) (Config, error) {
	get := func(name, def string) string {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v
		}
		return def
	}

	var (
		cfg  Config
		errs []error
		err  error
	)
	fail := func(name string, err error) {
		errs = append(errs, fmt.Errorf("%s: %w", name, err))
	}

	if cfg.Port, err = parsePort(get(EnvPort, "3000")); err != nil {
		fail(EnvPort, err)
	}
	if cfg.BackendURL, err = parseBackendURL(get(EnvBackendURL, "http://localhost:8080")); err != nil {
		fail(EnvBackendURL, err)
	}
	if cfg.BackendTimeout, err = parseDuration(get(EnvBackendTimeout, "5s")); err != nil {
		fail(EnvBackendTimeout, err)
	}
	if cfg.LogLevel, err = parseLevel(get(EnvLogLevel, "info")); err != nil {
		fail(EnvLogLevel, err)
	}
	if cfg.LogFormat, err = parseFormat(get(EnvLogFormat, LogFormatJSON)); err != nil {
		fail(EnvLogFormat, err)
	}
	if cfg.RequestTimeout, err = parseDuration(get(EnvRequestTimeout, "10s")); err != nil {
		fail(EnvRequestTimeout, err)
	}
	if cfg.ShutdownTimeout, err = parseDuration(get(EnvShutdownTimeout, "10s")); err != nil {
		fail(EnvShutdownTimeout, err)
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

// Logger builds a logger that writes to w in the configured level and format.
func (c Config) Logger(w io.Writer) *slog.Logger {
	return NewLogger(w, c.LogLevel, c.LogFormat)
}

// NewLogger builds a structured logger. Any format other than LogFormatText
// produces JSON, the production default.
func NewLogger(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == LogFormatText {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("%q is not a valid port; use a number from 1 to 65535", s)
	}
	return n, nil
}

func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid duration; use Go syntax such as 500ms, 5s or 1m", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%q must be greater than zero", s)
	}
	return d, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("%q is not a valid level; use debug, info, warn or error", s)
}

func parseFormat(s string) (string, error) {
	switch f := strings.ToLower(s); f {
	case LogFormatJSON, LogFormatText:
		return f, nil
	}
	return "", fmt.Errorf("%q is not a valid format; use json or text", s)
}

// parseBackendURL accepts an absolute http or https URL and returns it
// without a trailing slash, so the client can append absolute paths. Parts
// that would not survive that, such as a query string, are rejected rather
// than silently dropped.
func parseBackendURL(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid URL", s)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%q must be an absolute http or https URL", s)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("%q has no host", s)
	}
	if u.User != nil {
		return "", errors.New("the URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return "", fmt.Errorf("%q must not contain a query string or fragment", s)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}
