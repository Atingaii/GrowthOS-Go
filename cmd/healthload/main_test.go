package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunCLISuccess(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	exitCode := runCLI(context.Background(), []string{
		"-url=" + server.URL + "/health",
		"-rate=50",
		"-duration=80ms",
		"-workers=4",
		"-timeout=200ms",
		"-expected-status=200",
	}, stdout, stderr)

	if exitCode != 0 {
		t.Fatalf("runCLI exit code = %d, stderr = %q, stdout = %q", exitCode, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Count(stdout.String(), "\n") != 1 {
		t.Fatalf("stdout must be exactly one JSON line, got %q", stdout.String())
	}

	result := decodeReport(t, stdout.Bytes())
	if result.Target != server.URL+"/health" || result.StartedAt.IsZero() || result.FinishedAt.Before(result.StartedAt) {
		t.Fatalf("report identity/time fields are invalid: %+v", result)
	}
	if result.Method != http.MethodGet || result.EphemeralSelection {
		t.Fatalf("report method/mode = %q/%v, want GET/false", result.Method, result.EphemeralSelection)
	}
	if result.Scheduled != 4 || result.Completed != 4 || result.Success != 4 {
		t.Fatalf("request counts = scheduled:%d completed:%d success:%d, want 4/4/4", result.Scheduled, result.Completed, result.Success)
	}
	if result.Errors != 0 || result.UnexpectedStatus != 0 || result.Dropped != 0 {
		t.Fatalf("failure counts = errors:%d unexpected:%d dropped:%d, want zero", result.Errors, result.UnexpectedStatus, result.Dropped)
	}
	if result.StatusCounts["200"] != 4 {
		t.Fatalf("status_counts[200] = %d, want 4", result.StatusCounts["200"])
	}
	if result.ActualRPS <= 0 || result.MinMS < 0 || result.P50MS < result.MinMS || result.MaxMS < result.P99MS {
		t.Fatalf("invalid timing statistics: %+v", result)
	}
	if calls.Load() != 4 {
		t.Fatalf("server calls = %d, want 4", calls.Load())
	}
}

func TestRunCLIPOSTUsesConfiguredMethodWithEmptyBody(t *testing.T) {
	t.Parallel()

	type requestObservation struct {
		method           string
		body             []byte
		contentLength    int64
		transferEncoding []string
		accept           string
		userAgent        string
		demoMode         string
		readErr          error
	}

	var calls atomic.Int64
	observations := make(chan requestObservation, 4)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(request.Body)
		observations <- requestObservation{
			method:           request.Method,
			body:             body,
			contentLength:    request.ContentLength,
			transferEncoding: append([]string(nil), request.TransferEncoding...),
			accept:           request.Header.Get("Accept"),
			userAgent:        request.Header.Get("User-Agent"),
			demoMode:         request.Header.Get("X-GrowthOS-Demo-Mode"),
			readErr:          err,
		}
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	exitCode := runCLI(context.Background(), []string{
		"-url=" + server.URL + "/draw",
		"-method=POST",
		"-ephemeral-selection=true",
		"-rate=1",
		"-duration=1ms",
		"-workers=1",
		"-timeout=200ms",
	}, stdout, stderr)

	if exitCode != 0 {
		t.Fatalf("runCLI exit code = %d, stderr = %q, stdout = %q", exitCode, stderr.String(), stdout.String())
	}
	result := decodeReport(t, stdout.Bytes())
	if result.Method != http.MethodPost || !result.EphemeralSelection || result.Scheduled != 1 || result.Completed != 1 || result.Success != 1 {
		t.Fatalf("POST report = %+v", result)
	}

	observation := <-observations
	if observation.readErr != nil {
		t.Fatalf("read request body: %v", observation.readErr)
	}
	if observation.method != http.MethodPost {
		t.Fatalf("request method = %q, want %q", observation.method, http.MethodPost)
	}
	if len(observation.body) != 0 || observation.contentLength != 0 || len(observation.transferEncoding) != 0 {
		t.Fatalf(
			"POST body metadata = bytes:%d content-length:%d transfer-encoding:%v, want an empty body",
			len(observation.body),
			observation.contentLength,
			observation.transferEncoding,
		)
	}
	if observation.accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", observation.accept)
	}
	if observation.userAgent != userAgent {
		t.Fatalf("User-Agent = %q, want %q", observation.userAgent, userAgent)
	}
	if observation.demoMode != "ephemeral-selection" {
		t.Fatalf("X-GrowthOS-Demo-Mode = %q, want ephemeral-selection", observation.demoMode)
	}
	if calls.Load() != 1 {
		t.Fatalf("server calls = %d, want exactly one without retries", calls.Load())
	}
}

func TestMeasurePOSTDoesNotRetryTransportFailure(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.Method != http.MethodPost {
			t.Fatalf("request method = %q, want %q", request.Method, http.MethodPost)
		}
		if request.Body != nil {
			t.Fatal("POST request body must be nil")
		}
		if request.Header.Get("X-GrowthOS-Demo-Mode") != "ephemeral-selection" {
			t.Fatal("ephemeral selection acknowledgement is missing")
		}
		return nil, fmt.Errorf("forced transport failure")
	})}

	result := measure(context.Background(), client, http.MethodPost, "http://example.test/draw", true)
	if result.err == nil {
		t.Fatal("measure succeeded, want transport failure")
	}
	if calls.Load() != 1 {
		t.Fatalf("transport calls = %d, want exactly one without retries", calls.Load())
	}
}

func TestRunCLIUnexpectedStatusFailsWithoutLeakingBody(t *testing.T) {
	t.Parallel()

	const secretBody = "body-must-never-appear-in-output"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTeapot)
		_, _ = writer.Write([]byte(secretBody))
	}))
	t.Cleanup(server.Close)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	exitCode := runCLI(context.Background(), []string{
		"-url=" + server.URL,
		"-rate=1",
		"-duration=1ms",
		"-workers=1",
		"-timeout=200ms",
		"-expected-status=200",
	}, stdout, stderr)

	if exitCode != 1 {
		t.Fatalf("runCLI exit code = %d, want 1; stderr = %q", exitCode, stderr.String())
	}
	result := decodeReport(t, stdout.Bytes())
	if result.Scheduled != 1 || result.Completed != 1 || result.Success != 0 {
		t.Fatalf("request counts = scheduled:%d completed:%d success:%d, want 1/1/0", result.Scheduled, result.Completed, result.Success)
	}
	if result.Errors != 0 || result.UnexpectedStatus != 1 || result.StatusCounts["418"] != 1 {
		t.Fatalf("failure statistics = %+v", result)
	}
	if strings.Contains(stdout.String(), secretBody) || strings.Contains(stderr.String(), secretBody) {
		t.Fatal("response body leaked into command output")
	}
}

func TestRunCLITimeoutFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	exitCode := runCLI(context.Background(), []string{
		"-url=" + server.URL,
		"-rate=1",
		"-duration=1ms",
		"-workers=1",
		"-timeout=5ms",
	}, stdout, stderr)

	if exitCode != 1 {
		t.Fatalf("runCLI exit code = %d, want 1; stderr = %q", exitCode, stderr.String())
	}
	result := decodeReport(t, stdout.Bytes())
	if result.Scheduled != 1 || result.Completed != 1 || result.Errors != 1 || result.Success != 0 {
		t.Fatalf("timeout statistics = %+v", result)
	}
	if len(result.StatusCounts) != 0 || result.UnexpectedStatus != 0 {
		t.Fatalf("timeout must not fabricate an HTTP status: %+v", result)
	}
}

func TestRunCLIP99GateFailsWithoutTransportErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	exitCode := runCLI(context.Background(), []string{
		"-url=" + server.URL,
		"-rate=1",
		"-duration=1ms",
		"-workers=1",
		"-timeout=200ms",
		"-max-p99=1ms",
	}, stdout, stderr)

	if exitCode != 1 {
		t.Fatalf("runCLI exit code = %d, want 1; stderr = %q", exitCode, stderr.String())
	}
	result := decodeReport(t, stdout.Bytes())
	if result.Errors != 0 || result.UnexpectedStatus != 0 || !result.P99LimitExceeded {
		t.Fatalf("P99 gate result = %+v", result)
	}
	if result.MaxP99MS != 1 || result.P99MS <= result.MaxP99MS {
		t.Fatalf("P99 values did not preserve the failed threshold: %+v", result)
	}
}

func TestParseConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"-unknown=true"}},
		{name: "positional argument", args: []string{"extra"}},
		{name: "empty method", args: []string{"-method="}},
		{name: "lowercase method", args: []string{"-method=post"}},
		{name: "unsupported method", args: []string{"-method=PUT"}},
		{name: "ephemeral selection with GET", args: []string{"-method=GET", "-ephemeral-selection=true"}},
		{name: "relative URL", args: []string{"-url=/health"}},
		{name: "unsupported scheme", args: []string{"-url=ftp://example.com/health"}},
		{name: "URL user info", args: []string{"-url=http://user:password@example.com/health"}},
		{name: "URL query", args: []string{"-url=http://example.com/health?token=secret"}},
		{name: "zero rate", args: []string{"-rate=0"}},
		{name: "excessive rate", args: []string{fmt.Sprintf("-rate=%d", maxRate+1)}},
		{name: "zero duration", args: []string{"-duration=0s"}},
		{name: "excessive duration", args: []string{"-duration=25h"}},
		{name: "zero workers", args: []string{"-workers=0"}},
		{name: "excessive workers", args: []string{fmt.Sprintf("-workers=%d", maxWorkers+1)}},
		{name: "zero timeout", args: []string{"-timeout=0s"}},
		{name: "excessive timeout", args: []string{"-timeout=11m"}},
		{name: "low expected status", args: []string{"-expected-status=99"}},
		{name: "high expected status", args: []string{"-expected-status=600"}},
		{name: "negative P99 limit", args: []string{"-max-p99=-1ms"}},
		{name: "excessive P99 limit", args: []string{"-max-p99=11m"}},
		{name: "too many scheduled requests", args: []string{"-rate=100000", "-duration=2m"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseConfig(test.args); err == nil {
				t.Fatalf("parseConfig(%q) succeeded, want validation error", test.args)
			}
		})
	}
}

func TestParseConfigDefaultsAndScheduledCount(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig defaults: %v", err)
	}
	if cfg.URL != defaultURL || cfg.Method != http.MethodGet || cfg.EphemeralSelection || cfg.Rate != 100 || cfg.Duration != 5*time.Minute || cfg.Workers != 32 || cfg.Timeout != 2*time.Second || cfg.ExpectedStatus != 200 || cfg.MaxP99 != 0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	tests := []struct {
		rate     int
		duration time.Duration
		want     int64
	}{
		{rate: 100, duration: 5 * time.Minute, want: 30_000},
		{rate: 3, duration: time.Second, want: 3},
		{rate: 3, duration: time.Second + time.Nanosecond, want: 4},
		{rate: 1, duration: time.Nanosecond, want: 1},
	}
	for _, test := range tests {
		got, ok := scheduledRequestCount(test.rate, test.duration)
		if !ok || got != test.want {
			t.Errorf("scheduledRequestCount(%d, %s) = %d, %v; want %d, true", test.rate, test.duration, got, ok, test.want)
		}
	}
}

func TestPercentileBoundaries(t *testing.T) {
	t.Parallel()

	values := []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond, 4 * time.Millisecond}
	tests := []struct {
		name     string
		values   []time.Duration
		quantile float64
		want     time.Duration
	}{
		{name: "empty", values: nil, quantile: 0.50, want: 0},
		{name: "below zero", values: values, quantile: -1, want: time.Millisecond},
		{name: "zero", values: values, quantile: 0, want: time.Millisecond},
		{name: "p25", values: values, quantile: 0.25, want: time.Millisecond},
		{name: "p50 nearest rank", values: values, quantile: 0.50, want: 2 * time.Millisecond},
		{name: "p95", values: values, quantile: 0.95, want: 4 * time.Millisecond},
		{name: "one", values: values, quantile: 1, want: 4 * time.Millisecond},
		{name: "above one", values: values, quantile: 2, want: 4 * time.Millisecond},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := percentile(test.values, test.quantile); got != test.want {
				t.Fatalf("percentile(%v, %v) = %s, want %s", test.values, test.quantile, got, test.want)
			}
		})
	}
}

func TestRunCLIValidationWritesSingleJSONErrorLine(t *testing.T) {
	t.Parallel()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	if exitCode := runCLI(context.Background(), []string{"-rate=0"}, stdout, stderr); exitCode != 2 {
		t.Fatalf("runCLI exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if strings.Count(stderr.String(), "\n") != 1 {
		t.Fatalf("stderr must be one JSON line, got %q", stderr.String())
	}
	var errorResult struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &errorResult); err != nil {
		t.Fatalf("decode error JSON: %v", err)
	}
	if errorResult.Error == "" {
		t.Fatal("error JSON has an empty error message")
	}
}

func decodeReport(t *testing.T, data []byte) report {
	t.Helper()
	var result report
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode report %q: %v", data, err)
	}
	return result
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
