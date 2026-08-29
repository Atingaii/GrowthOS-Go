// Package httpserver owns the HTTP process lifecycle. It is separate from
// routing so that command packages only need to assemble dependencies.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultAddress           = ":8080"
	DefaultShutdownTimeout   = 5 * time.Second
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

// Config contains the validated listener and lifecycle values needed by the
// HTTP server. The process-level appconfig package owns environment loading;
// this package keeps defensive zero-value defaults for direct callers.
type Config struct {
	Address           string
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ErrorLogger       *slog.Logger
}

// Server runs one net/http server and stops it when its context is cancelled.
type Server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
}

// New creates an HTTP server with safe initial lifecycle defaults.
func New(handler http.Handler, config Config) *Server {
	address := strings.TrimSpace(config.Address)
	if address == "" {
		address = DefaultAddress
	}
	shutdownTimeout := config.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}
	readHeaderTimeout := config.ReadHeaderTimeout
	if readHeaderTimeout <= 0 {
		readHeaderTimeout = defaultReadHeaderTimeout
	}
	readTimeout := config.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = defaultReadTimeout
	}
	writeTimeout := config.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultWriteTimeout
	}
	idleTimeout := config.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultIdleTimeout
	}
	if handler == nil {
		handler = http.NotFoundHandler()
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			ErrorLog:          newHTTPErrorLog(config.ErrorLogger),
		},
		shutdownTimeout: shutdownTimeout,
	}
}

// Run binds the configured TCP address and serves until the context is
// cancelled. Bind failures are returned synchronously to the caller.
func (server *Server) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("run HTTP server: context is nil")
	}
	if ctx.Err() != nil {
		return nil
	}

	listener, err := net.Listen("tcp", server.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", server.httpServer.Addr, err)
	}

	return server.Serve(ctx, listener)
}

// Serve runs on an already-bound listener. Accepting a listener makes graceful
// shutdown behavior testable without racing for a free port.
func (server *Server) Serve(ctx context.Context, listener net.Listener) error {
	if ctx == nil {
		return errors.New("serve HTTP: context is nil")
	}
	if listener == nil {
		return errors.New("serve HTTP: listener is nil")
	}
	if ctx.Err() != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("close listener for cancelled context: %w", err)
		}
		return nil
	}

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.httpServer.Serve(listener)
	}()

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		return server.shutdown(serveErrors)
	}
}

func (server *Server) shutdown(serveErrors <-chan error) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
	defer cancel()
	shutdownErr := server.httpServer.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		closeErr := server.httpServer.Close()
		serveErr := <-serveErrors
		return errors.Join(
			fmt.Errorf("shut down HTTP server: %w", shutdownErr),
			wrapError("force close HTTP server", closeErr),
			wrapServeError(serveErr),
		)
	}

	serveErr := <-serveErrors
	return wrapServeError(serveErr)
}

func wrapError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func wrapServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return fmt.Errorf("serve HTTP while shutting down: %w", err)
}
