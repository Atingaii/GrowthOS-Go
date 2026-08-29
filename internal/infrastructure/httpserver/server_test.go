package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewAppliesSafeHTTPTimeoutDefaults(t *testing.T) {
	server := New(http.NotFoundHandler(), Config{})

	if server.httpServer.ErrorLog == nil {
		t.Fatal("net/http ErrorLog = nil, want safe non-global logger")
	}

	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "read header", got: server.httpServer.ReadHeaderTimeout, want: 5 * time.Second},
		{name: "read", got: server.httpServer.ReadTimeout, want: 15 * time.Second},
		{name: "write", got: server.httpServer.WriteTimeout, want: 30 * time.Second},
		{name: "idle", got: server.httpServer.IdleTimeout, want: 60 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("timeout = %s, want %s", test.got, test.want)
			}
		})
	}
}

func TestNewRoutesNetHTTPDiagnosticsThroughRedactedStructuredLog(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	server := New(http.NotFoundHandler(), Config{ErrorLogger: logger})

	server.httpServer.ErrorLog.Print("panic serving: password=must-not-appear\nstack-secret")

	if strings.Contains(output.String(), "must-not-appear") || strings.Contains(output.String(), "stack-secret") {
		t.Fatalf("net/http error log leaked raw diagnostic: %s", output.String())
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode structured net/http log: %v\n%s", err, output.String())
	}
	if record["level"] != "ERROR" || record["msg"] != "http_server_error" || record["component"] != "net/http" {
		t.Fatalf("unexpected net/http log record: %#v", record)
	}
}

func TestNewAppliesConfiguredHTTPTimeouts(t *testing.T) {
	server := New(http.NotFoundHandler(), Config{
		ShutdownTimeout:   7 * time.Second,
		ReadHeaderTimeout: 8 * time.Second,
		ReadTimeout:       9 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       11 * time.Second,
	})

	if server.shutdownTimeout != 7*time.Second {
		t.Fatalf("shutdown timeout = %s, want %s", server.shutdownTimeout, 7*time.Second)
	}
	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "read header", got: server.httpServer.ReadHeaderTimeout, want: 8 * time.Second},
		{name: "read", got: server.httpServer.ReadTimeout, want: 9 * time.Second},
		{name: "write", got: server.httpServer.WriteTimeout, want: 10 * time.Second},
		{name: "idle", got: server.httpServer.IdleTimeout, want: 11 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("timeout = %s, want %s", test.got, test.want)
			}
		})
	}
}

func TestRunReturnsListenFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	defer occupied.Close()

	server := New(http.NotFoundHandler(), Config{Address: occupied.Addr().String()})
	err = server.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want listen failure")
	}
	if !strings.Contains(err.Error(), "listen on ") {
		t.Fatalf("Run() error = %q, want listen context", err)
	}
	var operationError *net.OpError
	if !errors.As(err, &operationError) {
		t.Fatalf("Run() error = %T, want wrapped *net.OpError", err)
	}
}

func TestServeReturnsListenerFailure(t *testing.T) {
	wantErr := errors.New("accept failed")
	server := New(http.NotFoundHandler(), Config{})

	err := server.Serve(context.Background(), failingListener{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Serve() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestServeGracefullyShutsDownAfterContextCancellation(t *testing.T) {
	rawListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	listener := newCloseNotifyingListener(rawListener)

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseRequest) })
	}
	defer release()

	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		response.WriteHeader(http.StatusNoContent)
	})
	server := New(handler, Config{ShutdownTimeout: 2 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(ctx, listener)
	}()

	type clientResult struct {
		statusCode int
		err        error
	}
	clientDone := make(chan clientResult, 1)
	go func() {
		response, requestErr := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + listener.Addr().String())
		if requestErr != nil {
			clientDone <- clientResult{err: requestErr}
			return
		}
		_, copyErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		clientDone <- clientResult{
			statusCode: response.StatusCode,
			err:        errors.Join(copyErr, closeErr),
		}
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach handler")
	}

	cancel()
	select {
	case <-listener.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop accepting new connections")
	}
	select {
	case err := <-serveDone:
		t.Fatalf("Serve() returned after closing the listener but before active request completed: %v", err)
	default:
	}

	release()
	select {
	case result := <-clientDone:
		if result.err != nil {
			t.Fatalf("client request: %v", result.err)
		}
		if result.statusCode != http.StatusNoContent {
			t.Fatalf("status code = %d, want %d", result.statusCode, http.StatusNoContent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active request did not complete")
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish graceful shutdown")
	}
}

func TestServeReturnsShutdownTimeoutAndForceClosesConnections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	requestStarted := make(chan struct{})
	requestStopped := make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
		close(requestStopped)
	})
	server := New(handler, Config{ShutdownTimeout: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(ctx, listener)
	}()
	clientDone := make(chan error, 1)
	go func() {
		response, requestErr := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + listener.Addr().String())
		if requestErr != nil {
			clientDone <- requestErr
			return
		}
		_, copyErr := io.Copy(io.Discard, response.Body)
		clientDone <- errors.Join(copyErr, response.Body.Close())
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach handler")
	}
	cancel()

	select {
	case err := <-serveDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Serve() error = %v, want shutdown deadline exceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not return after shutdown timeout")
	}

	select {
	case <-requestStopped:
	case <-time.After(2 * time.Second):
		t.Fatal("forced close did not cancel active request")
	}
	select {
	case <-clientDone:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not observe forced close")
	}
}

type failingListener struct {
	err error
}

func (listener failingListener) Accept() (net.Conn, error) {
	return nil, listener.err
}

func (failingListener) Close() error {
	return nil
}

func (failingListener) Addr() net.Addr {
	return testAddress("failing-listener")
}

type testAddress string

type closeNotifyingListener struct {
	net.Listener
	closed   chan struct{}
	closeErr error
	close    sync.Once
}

func newCloseNotifyingListener(listener net.Listener) *closeNotifyingListener {
	return &closeNotifyingListener{
		Listener: listener,
		closed:   make(chan struct{}),
	}
}

func (listener *closeNotifyingListener) Close() error {
	listener.close.Do(func() {
		listener.closeErr = listener.Listener.Close()
		close(listener.closed)
	})
	return listener.closeErr
}

func (address testAddress) Network() string {
	return string(address)
}

func (address testAddress) String() string {
	return string(address)
}
