// Package logging constructs the process-wide structured logger.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
)

// New creates a structured slog logger with validated level and format.
func New(output io.Writer, level, format string, baseAttrs ...slog.Attr) (*slog.Logger, error) {
	if nilWriter(output) {
		return nil, fmt.Errorf("create logger: output is nil")
	}

	parsedLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	options := &slog.HandlerOptions{Level: parsedLevel}
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("create logger: unsupported format %q (want json or text)", format)
	}

	if len(baseAttrs) > 0 {
		handler = handler.WithAttrs(baseAttrs)
	}
	return slog.New(handler), nil
}

func nilWriter(output io.Writer) bool {
	if output == nil {
		return true
	}
	value := reflect.ValueOf(output)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("create logger: unsupported level %q (want debug, info, warn, or error)", value)
	}
}
