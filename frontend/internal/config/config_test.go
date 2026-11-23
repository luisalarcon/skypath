package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestLoadFromDefaults(t *testing.T) {
	got, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	want := Config{
		Port:            3000,
		BackendURL:      "http://localhost:8080",
		BackendTimeout:  5 * time.Second,
		LogLevel:        slog.LevelInfo,
		LogFormat:       "json",
		RequestTimeout:  10 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
	if got != want {
		t.Errorf("defaults = %+v, want %+v", got, want)
	}
	if got.Addr() != ":3000" {
		t.Errorf("Addr() = %q, want :3000", got.Addr())
	}
}

func TestLoadFromBlankValuesUseDefaults(t *testing.T) {
	got, err := LoadFrom(env(map[string]string{EnvPort: "  ", EnvBackendURL: ""}))
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if got.Port != 3000 || got.BackendURL != "http://localhost:8080" {
		t.Errorf("blank values should fall back to defaults, got port %d url %q", got.Port, got.BackendURL)
	}
}

func TestLoadFromOverrides(t *testing.T) {
	tests := []struct {
		name  string
		vars  map[string]string
		check func(Config) bool
	}{
		{"port", map[string]string{EnvPort: "8081"}, func(c Config) bool { return c.Port == 8081 }},
		{"backend url", map[string]string{EnvBackendURL: "https://api.example.com"}, func(c Config) bool { return c.BackendURL == "https://api.example.com" }},
		{"backend url trailing slash", map[string]string{EnvBackendURL: "http://api:8080/"}, func(c Config) bool { return c.BackendURL == "http://api:8080" }},
		{"backend url with path prefix", map[string]string{EnvBackendURL: "http://proxy/skypath/"}, func(c Config) bool { return c.BackendURL == "http://proxy/skypath" }},
		{"backend timeout", map[string]string{EnvBackendTimeout: "750ms"}, func(c Config) bool { return c.BackendTimeout == 750*time.Millisecond }},
		{"log level debug", map[string]string{EnvLogLevel: "debug"}, func(c Config) bool { return c.LogLevel == slog.LevelDebug }},
		{"log level warn", map[string]string{EnvLogLevel: "warn"}, func(c Config) bool { return c.LogLevel == slog.LevelWarn }},
		{"log level error", map[string]string{EnvLogLevel: "error"}, func(c Config) bool { return c.LogLevel == slog.LevelError }},
		{"log level case", map[string]string{EnvLogLevel: "DEBUG"}, func(c Config) bool { return c.LogLevel == slog.LevelDebug }},
		{"log format text", map[string]string{EnvLogFormat: "text"}, func(c Config) bool { return c.LogFormat == "text" }},
		{"request timeout", map[string]string{EnvRequestTimeout: "1m"}, func(c Config) bool { return c.RequestTimeout == time.Minute }},
		{"shutdown timeout", map[string]string{EnvShutdownTimeout: "30s"}, func(c Config) bool { return c.ShutdownTimeout == 30*time.Second }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadFrom(env(tt.vars))
			if err != nil {
				t.Fatalf("LoadFrom returned error: %v", err)
			}
			if !tt.check(got) {
				t.Errorf("override not applied: %+v", got)
			}
		})
	}
}

func TestLoadFromInvalidValuesNameTheVariable(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{EnvPort, "abc"},
		{EnvPort, "0"},
		{EnvPort, "70000"},
		{EnvPort, "-1"},
		{EnvBackendURL, "localhost:8080"},
		{EnvBackendURL, "/relative"},
		{EnvBackendURL, "ftp://example.com"},
		{EnvBackendURL, "http://"},
		{EnvBackendURL, "http://:8080"},
		{EnvBackendURL, "http://user:pass@example.com"},
		{EnvBackendURL, "http://example.com/?x=1"},
		{EnvBackendURL, "http://example.com/#frag"},
		{EnvBackendURL, "http://exa mple.com"},
		{EnvBackendTimeout, "soon"},
		{EnvBackendTimeout, "5"},
		{EnvBackendTimeout, "0s"},
		{EnvBackendTimeout, "-5s"},
		{EnvLogLevel, "verbose"},
		{EnvLogFormat, "xml"},
		{EnvRequestTimeout, "0"},
		{EnvShutdownTimeout, "forever"},
	}
	for _, tt := range tests {
		t.Run(tt.variable+"="+tt.value, func(t *testing.T) {
			_, err := LoadFrom(env(map[string]string{tt.variable: tt.value}))
			if err == nil {
				t.Fatalf("LoadFrom accepted %s=%q", tt.variable, tt.value)
			}
			if !strings.Contains(err.Error(), tt.variable+":") {
				t.Errorf("error %q does not name %s", err, tt.variable)
			}
		})
	}
}

func TestLoadFromReportsEveryInvalidValue(t *testing.T) {
	_, err := LoadFrom(env(map[string]string{EnvPort: "x", EnvLogLevel: "loud"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, name := range []string{EnvPort, EnvLogLevel} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not mention %s", err, name)
		}
	}
}

func TestLoadReadsProcessEnvironment(t *testing.T) {
	t.Setenv(EnvPort, "4000")
	got, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.Port != 4000 {
		t.Errorf("Port = %d, want 4000", got.Port)
	}
}

func TestLogger(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		prefix string
	}{
		{"json", Config{LogFormat: "json", LogLevel: slog.LevelInfo}, `{"time"`},
		{"text", Config{LogFormat: "text", LogLevel: slog.LevelInfo}, "time="},
		{"zero value defaults to json", Config{}, `{"time"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.cfg.Logger(&buf).Info("hello")
			if !strings.HasPrefix(buf.String(), tt.prefix) {
				t.Errorf("output %q does not start with %q", buf.String(), tt.prefix)
			}
		})
	}
}

func TestLoggerHonoursLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelWarn, "json")
	logger.Info("quiet")
	if buf.Len() != 0 {
		t.Errorf("info was logged at warn level: %q", buf.String())
	}
	logger.Warn("loud")
	if !strings.Contains(buf.String(), "loud") {
		t.Errorf("warn was not logged: %q", buf.String())
	}
}
