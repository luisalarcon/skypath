// Package config reads the backend's settings from environment variables.
//
// Every value is validated once, at startup, so a typo surfaces as an error
// naming the variable instead of as odd behaviour later. The variables and
// their defaults are documented in docs/operations.md.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variable names.
const (
	EnvPort            = "PORT"
	EnvFlightsPath     = "FLIGHTS_PATH"
	EnvLogLevel        = "LOG_LEVEL"
	EnvLogFormat       = "LOG_FORMAT"
	EnvRequestTimeout  = "REQUEST_TIMEOUT"
	EnvShutdownTimeout = "SHUTDOWN_TIMEOUT"
)

// Defaults used when a variable is unset or empty.
const (
	DefaultPort            = 8080
	DefaultFlightsPath     = "flights.json"
	DefaultLogFormat       = FormatJSON
	DefaultRequestTimeout  = 10 * time.Second
	DefaultShutdownTimeout = 10 * time.Second
)

// Log output formats accepted by LOG_FORMAT.
const (
	FormatJSON = "json"
	FormatText = "text"
)

// Config holds the validated backend settings.
type Config struct {
	Port            int           // TCP port to listen on, 1 to 65535
	FlightsPath     string        // path of the schedule file
	LogLevel        slog.Level    // minimum level that is logged
	LogFormat       string        // FormatJSON or FormatText
	RequestTimeout  time.Duration // time limit for handling one request
	ShutdownTimeout time.Duration // time allowed for in-flight requests on shutdown
}

// Addr returns the listen address for http.Server.
func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}

// NewLogger returns a logger that writes to w in the configured format and
// at the configured level.
func (c Config) NewLogger(w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	if c.LogFormat == FormatText {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// Load reads the configuration from the process environment. If several
// variables are invalid, the returned error reports all of them, one per
// line, each naming its variable.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

// load is Load with the environment passed in, so tests need not touch the
// real one.
func load(lookup func(string) (string, bool)) (Config, error) {
	// An empty value counts as unset: container platforms commonly pass
	// through variables that were never given a value.
	get := func(name string) (string, bool) {
		v, ok := lookup(name)
		return v, ok && v != ""
	}

	cfg := Config{
		Port:            DefaultPort,
		FlightsPath:     DefaultFlightsPath,
		LogLevel:        slog.LevelInfo,
		LogFormat:       DefaultLogFormat,
		RequestTimeout:  DefaultRequestTimeout,
		ShutdownTimeout: DefaultShutdownTimeout,
	}
	var errs []error
	fail := func(name, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", name, fmt.Sprintf(format, args...)))
	}

	if v, ok := get(EnvPort); ok {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			fail(EnvPort, "must be an integer from 1 to 65535, got %q", v)
		} else {
			cfg.Port = port
		}
	}

	if v, ok := get(EnvFlightsPath); ok {
		if strings.TrimSpace(v) == "" {
			fail(EnvFlightsPath, "must not be blank")
		} else {
			cfg.FlightsPath = v
		}
	}

	if v, ok := get(EnvLogLevel); ok {
		level, err := parseLevel(v)
		if err != nil {
			fail(EnvLogLevel, "%v", err)
		} else {
			cfg.LogLevel = level
		}
	}

	if v, ok := get(EnvLogFormat); ok {
		switch format := strings.ToLower(v); format {
		case FormatJSON, FormatText:
			cfg.LogFormat = format
		default:
			fail(EnvLogFormat, "must be %q or %q, got %q", FormatJSON, FormatText, v)
		}
	}

	for _, d := range []struct {
		name string
		dst  *time.Duration
	}{
		{EnvRequestTimeout, &cfg.RequestTimeout},
		{EnvShutdownTimeout, &cfg.ShutdownTimeout},
	} {
		v, ok := get(d.name)
		if !ok {
			continue
		}
		parsed, err := time.ParseDuration(v)
		if err != nil || parsed <= 0 {
			fail(d.name, "must be a positive duration such as 500ms, 5s or 1m, got %q", v)
			continue
		}
		*d.dst = parsed
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
	return 0, fmt.Errorf("must be debug, info, warn or error, got %q", s)
}
