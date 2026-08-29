package httpserver

import (
	"io"
	"log"
	"log/slog"
)

// newHTTPErrorLog prevents net/http from falling back to the process-wide
// standard logger. Raw net/http diagnostics can include panic values and
// stacks, so the bridge emits only a stable structured event. Serve failures
// still return through Server.Run and are logged by the process entry point.
func newHTTPErrorLog(logger *slog.Logger) *log.Logger {
	var output io.Writer = io.Discard
	if logger != nil {
		output = httpErrorLogWriter{logger: logger}
	}
	return log.New(output, "", 0)
}

type httpErrorLogWriter struct {
	logger *slog.Logger
}

func (writer httpErrorLogWriter) Write(message []byte) (int, error) {
	writer.logger.Error(
		"http_server_error",
		slog.String("component", "net/http"),
	)
	return len(message), nil
}
