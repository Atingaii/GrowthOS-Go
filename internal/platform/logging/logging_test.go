package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestNewWritesJSONWithBaseAttributes(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(
		&output,
		"debug",
		"json",
		slog.String("service", "growth-api"),
		slog.String("version", "lesson-12-test"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Debug("configuration loaded", slog.String("component", "test"))

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode JSON log: %v\n%s", err, output.String())
	}
	assertJSONField(t, record, "level", "DEBUG")
	assertJSONField(t, record, "msg", "configuration loaded")
	assertJSONField(t, record, "service", "growth-api")
	assertJSONField(t, record, "version", "lesson-12-test")
	assertJSONField(t, record, "component", "test")
}

func TestNewAppliesConfiguredLevel(t *testing.T) {
	levels := []struct {
		configured string
		emit       func(*slog.Logger, string)
		visible    string
	}{
		{configured: "debug", emit: func(logger *slog.Logger, message string) { logger.Debug(message) }, visible: "debug-visible"},
		{configured: "info", emit: func(logger *slog.Logger, message string) { logger.Info(message) }, visible: "info-visible"},
		{configured: "warn", emit: func(logger *slog.Logger, message string) { logger.Warn(message) }, visible: "warn-visible"},
		{configured: "error", emit: func(logger *slog.Logger, message string) { logger.Error(message) }, visible: "error-visible"},
	}

	for _, test := range levels {
		t.Run(test.configured, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := New(&output, test.configured, "json")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			logger.Debug("debug-hidden-at-higher-levels")
			test.emit(logger, test.visible)
			if !strings.Contains(output.String(), test.visible) {
				t.Fatalf("log output %q does not contain %q", output.String(), test.visible)
			}
			if test.configured != "debug" && strings.Contains(output.String(), "debug-hidden-at-higher-levels") {
				t.Fatalf("debug log was not filtered: %s", output.String())
			}
		})
	}
}

func TestNewSupportsTextFormat(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, "INFO", " TEXT ", slog.String("service", "growth-api"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Warn("visible warning", slog.String("request_id", "request-1"))

	logLine := output.String()
	for _, fragment := range []string{"level=WARN", `msg="visible warning"`, "service=growth-api", "request_id=request-1"} {
		if !strings.Contains(logLine, fragment) {
			t.Fatalf("text log %q does not contain %q", logLine, fragment)
		}
	}
}

func TestNewRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name   string
		output *bytes.Buffer
		level  string
		format string
	}{
		{name: "nil output", output: nil, level: "info", format: "json"},
		{name: "level", output: &bytes.Buffer{}, level: "trace", format: "json"},
		{name: "empty level", output: &bytes.Buffer{}, level: "", format: "json"},
		{name: "format", output: &bytes.Buffer{}, level: "info", format: "yaml"},
		{name: "empty format", output: &bytes.Buffer{}, level: "info", format: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.output, test.level, test.format); err == nil {
				t.Fatal("New() error = nil, want validation failure")
			}
		})
	}
}

func TestNewAcceptsNonNilValueWriter(t *testing.T) {
	logger, err := New(io.Discard, "info", "json")
	if err != nil {
		t.Fatalf("New(io.Discard) error = %v", err)
	}
	logger.Info("discarded safely")
}

func TestNewRejectsNilWriterInterface(t *testing.T) {
	if _, err := New(nil, "info", "json"); err == nil {
		t.Fatal("New(nil) error = nil, want validation failure")
	}

	var typedNil *bytes.Buffer
	if _, err := New(typedNil, "info", "json"); err == nil {
		t.Fatal("New(typed nil) error = nil, want validation failure")
	}
}

func assertJSONField(t *testing.T, record map[string]any, key string, want any) {
	t.Helper()
	if got := record[key]; got != want {
		t.Fatalf("JSON log field %q = %#v, want %#v", key, got, want)
	}
}
