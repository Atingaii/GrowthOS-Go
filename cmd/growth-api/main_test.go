package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/gin-gonic/gin"
)

func TestRunRejectsInvalidConfigurationWithoutEchoingValue(t *testing.T) {
	var output bytes.Buffer
	secretValue := "secret-invalid-level"
	exitCode := run(
		context.Background(),
		mapLookup(map[string]string{"GROWTHOS_LOG_LEVEL": secretValue}),
		&output,
	)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if strings.Contains(output.String(), secretValue) {
		t.Fatalf("configuration log echoed rejected value: %s", output.String())
	}

	entry := decodeJSONLog(t, output.String())
	if entry["level"] != "ERROR" || entry["msg"] != "configuration rejected" {
		t.Fatalf("unexpected bootstrap log: %#v", entry)
	}
	if entry["service"] != serviceName || entry["version"] != version {
		t.Fatalf("bootstrap identity fields = %#v", entry)
	}
}

func TestRunLogsLifecycleWithValidatedConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	variables := map[string]string{
		"GROWTHOS_ENVIRONMENT":  "test",
		"GROWTHOS_HTTP_ADDRESS": "127.0.0.1:9090",
		"GROWTHOS_LOG_LEVEL":    "info",
		"GROWTHOS_LOG_FORMAT":   "json",
	}

	if exitCode := run(ctx, mapLookup(variables), &output); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got := gin.Mode(); got != gin.ReleaseMode {
		t.Fatalf("Gin mode = %q, want %q so stdout remains structured", got, gin.ReleaseMode)
	}

	lines := nonEmptyLines(output.String())
	if len(lines) != 2 {
		t.Fatalf("log line count = %d, want 2; output = %q", len(lines), output.String())
	}
	started := decodeJSONLog(t, lines[0])
	stopped := decodeJSONLog(t, lines[1])
	if started["msg"] != "service starting" || stopped["msg"] != "service stopped" {
		t.Fatalf("lifecycle messages = %q, %q", started["msg"], stopped["msg"])
	}
	if started["service"] != serviceName || started["environment"] != "test" || started["version"] != version {
		t.Fatalf("service identity fields = %#v", started)
	}
	if started["http_address"] != "127.0.0.1:9090" || started["shutdown_timeout_ms"] != float64(5000) {
		t.Fatalf("service configuration fields = %#v", started)
	}
}

func TestRunHonorsErrorLogLevel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	variables := map[string]string{
		"GROWTHOS_LOG_LEVEL":  "error",
		"GROWTHOS_LOG_FORMAT": "json",
	}

	if exitCode := run(ctx, mapLookup(variables), &output); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if output.Len() != 0 {
		t.Fatalf("error-level logger emitted info lifecycle logs: %q", output.String())
	}
}

func TestRunRejectsNilOutput(t *testing.T) {
	if exitCode := run(context.Background(), mapLookup(nil), nil); exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}

	var typedNil *bytes.Buffer
	if exitCode := run(context.Background(), mapLookup(nil), typedNil); exitCode != 1 {
		t.Fatalf("run() typed-nil exit code = %d, want 1", exitCode)
	}
}

func TestHTTPServerConfigMapsEveryValidatedSetting(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	input := appconfig.HTTPConfig{
		Address:           "127.0.0.1:18080",
		ShutdownTimeout:   1 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       5 * time.Second,
	}

	got := httpServerConfig(input, logger)
	if got.Address != input.Address ||
		got.ShutdownTimeout != input.ShutdownTimeout ||
		got.ReadHeaderTimeout != input.ReadHeaderTimeout ||
		got.ReadTimeout != input.ReadTimeout ||
		got.WriteTimeout != input.WriteTimeout ||
		got.IdleTimeout != input.IdleTimeout ||
		got.ErrorLogger != logger {
		t.Fatalf("httpServerConfig() = %#v, want all validated values and logger", got)
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func decodeJSONLog(t *testing.T, line string) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &entry); err != nil {
		t.Fatalf("decode log %q: %v", line, err)
	}
	return entry
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
