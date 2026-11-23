package config

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// env builds a lookup function over a fixed set of variables.
func env(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	got, err := load(env(nil))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	want := Config{
		Port:            8080,
		FlightsPath:     "flights.json",
		LogLevel:        slog.LevelInfo,
		LogFormat:       "json",
		RequestTimeout:  10 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
	if got != want {
		t.Errorf("defaults = %+v, want %+v", got, want)
	}
	if got.Addr() != ":8080" {
		t.Errorf("Addr = %q, want :8080", got.Addr())
	}
}

func TestLoadEmptyValueUsesDefault(t *testing.T) {
	got, err := load(env(map[string]string{"PORT": "", "LOG_LEVEL": "", "REQUEST_TIMEOUT": ""}))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if got.Port != DefaultPort || got.LogLevel != slog.LevelInfo || got.RequestTimeout != DefaultRequestTimeout {
		t.Errorf("empty values should fall back to defaults, got %+v", got)
	}
}

func TestLoadOverrides(t *testing.T) {
	tests := []struct {
		name  string
		vars  map[string]string
		check func(Config) bool
	}{
		{"port", map[string]string{"PORT": "9090"}, func(c Config) bool { return c.Port == 9090 && c.Addr() == ":9090" }},
		{"port lower bound", map[string]string{"PORT": "1"}, func(c Config) bool { return c.Port == 1 }},
		{"port upper bound", map[string]string{"PORT": "65535"}, func(c Config) bool { return c.Port == 65535 }},
		{"flights path", map[string]string{"FLIGHTS_PATH": "../flights.json"}, func(c Config) bool { return c.FlightsPath == "../flights.json" }},
		{"level debug", map[string]string{"LOG_LEVEL": "debug"}, func(c Config) bool { return c.LogLevel == slog.LevelDebug }},
		{"level warn", map[string]string{"LOG_LEVEL": "warn"}, func(c Config) bool { return c.LogLevel == slog.LevelWarn }},
		{"level error", map[string]string{"LOG_LEVEL": "error"}, func(c Config) bool { return c.LogLevel == slog.LevelError }},
		{"level is case-insensitive", map[string]string{"LOG_LEVEL": "WARN"}, func(c Config) bool { return c.LogLevel == slog.LevelWarn }},
		{"format text", map[string]string{"LOG_FORMAT": "text"}, func(c Config) bool { return c.LogFormat == "text" }},
		{"format is case-insensitive", map[string]string{"LOG_FORMAT": "TEXT"}, func(c Config) bool { return c.LogFormat == "text" }},
		{"request timeout", map[string]string{"REQUEST_TIMEOUT": "500ms"}, func(c Config) bool { return c.RequestTimeout == 500*time.Millisecond }},
		{"shutdown timeout", map[string]string{"SHUTDOWN_TIMEOUT": "1m"}, func(c Config) bool { return c.ShutdownTimeout == time.Minute }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := load(env(tc.vars))
			if err != nil {
				t.Fatalf("load returned error: %v", err)
			}
			if !tc.check(got) {
				t.Errorf("override not applied: %+v", got)
			}
		})
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{"PORT", "abc"},
		{"PORT", "0"},
		{"PORT", "-1"},
		{"PORT", "65536"},
		{"PORT", "80.5"},
		{"PORT", " 8080"},
		{"FLIGHTS_PATH", "   "},
		{"LOG_LEVEL", "verbose"},
		{"LOG_LEVEL", "info+2"},
		{"LOG_FORMAT", "yaml"},
		{"REQUEST_TIMEOUT", "10"},
		{"REQUEST_TIMEOUT", "soon"},
		{"REQUEST_TIMEOUT", "0s"},
		{"REQUEST_TIMEOUT", "-5s"},
		{"SHUTDOWN_TIMEOUT", "ten seconds"},
		{"SHUTDOWN_TIMEOUT", "0"},
		{"SHUTDOWN_TIMEOUT", "-1m"},
	}
	for _, tc := range tests {
		t.Run(tc.variable+"="+tc.value, func(t *testing.T) {
			_, err := load(env(map[string]string{tc.variable: tc.value}))
			if err == nil {
				t.Fatalf("load accepted %s=%q", tc.variable, tc.value)
			}
			if !strings.Contains(err.Error(), tc.variable) {
				t.Errorf("error %q does not name %s", err, tc.variable)
			}
		})
	}
}

func TestLoadReportsEveryInvalidVariable(t *testing.T) {
	_, err := load(env(map[string]string{"PORT": "x", "LOG_LEVEL": "loud", "REQUEST_TIMEOUT": "never"}))
	if err == nil {
		t.Fatal("load accepted invalid values")
	}
	for _, name := range []string{"PORT", "LOG_LEVEL", "REQUEST_TIMEOUT"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestNewLogger(t *testing.T) {
	t.Run("json at level", func(t *testing.T) {
		var buf bytes.Buffer
		logger := Config{LogLevel: slog.LevelWarn, LogFormat: FormatJSON}.NewLogger(&buf)
		logger.Info("hidden")
		logger.Warn("shown", "k", "v")

		var line map[string]any
		if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
			t.Fatalf("output is not a single JSON object: %q: %v", buf.String(), err)
		}
		if line["msg"] != "shown" || line["level"] != "WARN" || line["k"] != "v" {
			t.Errorf("unexpected log line: %v", line)
		}
	})
	t.Run("text", func(t *testing.T) {
		var buf bytes.Buffer
		Config{LogLevel: slog.LevelInfo, LogFormat: FormatText}.NewLogger(&buf).Info("hello", "k", "v")
		if got := buf.String(); !strings.Contains(got, "msg=hello") || !strings.Contains(got, "k=v") {
			t.Errorf("text output = %q", got)
		}
	})
}
