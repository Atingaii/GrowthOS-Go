package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	defaultURL            = "http://127.0.0.1:8088/health"
	defaultRate           = 100
	defaultDuration       = 5 * time.Minute
	defaultWorkers        = 32
	defaultTimeout        = 2 * time.Second
	defaultExpectedStatus = http.StatusOK
	defaultMethod         = http.MethodGet
	userAgent             = "growthos-healthload/1"

	maxRate              = 100_000
	maxDuration          = 24 * time.Hour
	maxWorkers           = 4_096
	maxTimeout           = 10 * time.Minute
	maxScheduledRequests = 10_000_000
	maxResponseBodyBytes = 4 << 10
)

type config struct {
	URL            string
	Method         string
	Rate           int
	Duration       time.Duration
	Workers        int
	Timeout        time.Duration
	ExpectedStatus int
	MaxP99         time.Duration
}

type report struct {
	Target           string           `json:"target"`
	Method           string           `json:"method"`
	StartedAt        time.Time        `json:"started_at"`
	FinishedAt       time.Time        `json:"finished_at"`
	Rate             int              `json:"rate"`
	DurationMS       float64          `json:"duration_ms"`
	Workers          int              `json:"workers"`
	TimeoutMS        float64          `json:"timeout_ms"`
	ExpectedStatus   int              `json:"expected_status"`
	Scheduled        int64            `json:"scheduled"`
	Completed        int64            `json:"completed"`
	Success          int64            `json:"success"`
	Errors           int64            `json:"errors"`
	UnexpectedStatus int64            `json:"unexpected_status"`
	Dropped          int64            `json:"dropped"`
	StatusCounts     map[string]int64 `json:"status_counts"`
	ActualRPS        float64          `json:"actual_rps"`
	ElapsedMS        float64          `json:"elapsed_ms"`
	MinMS            float64          `json:"min_ms"`
	P50MS            float64          `json:"p50_ms"`
	P95MS            float64          `json:"p95_ms"`
	P99MS            float64          `json:"p99_ms"`
	MaxMS            float64          `json:"max_ms"`
	MaxP99MS         float64          `json:"max_p99_ms"`
	P99LimitExceeded bool             `json:"p99_limit_exceeded"`
}

func (r report) failed() bool {
	return r.Errors != 0 || r.UnexpectedStatus != 0 || r.Completed != r.Scheduled || r.P99LimitExceeded
}

type measurement struct {
	status  int
	latency time.Duration
	err     error
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args)
	if err != nil {
		_ = writeJSONLine(stderr, struct {
			Error string `json:"error"`
		}{Error: err.Error()})
		return 2
	}

	result := runLoad(ctx, cfg, newHTTPClient(cfg))
	if err := writeJSONLine(stdout, result); err != nil {
		_ = writeJSONLine(stderr, struct {
			Error string `json:"error"`
		}{Error: "write result: " + err.Error()})
		return 2
	}
	if result.failed() {
		return 1
	}
	return 0
}

func parseConfig(args []string) (config, error) {
	cfg := config{}
	flags := flag.NewFlagSet("healthload", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.URL, "url", defaultURL, "health endpoint URL")
	flags.StringVar(&cfg.Method, "method", defaultMethod, "HTTP method (GET or POST)")
	flags.IntVar(&cfg.Rate, "rate", defaultRate, "scheduled requests per second")
	flags.DurationVar(&cfg.Duration, "duration", defaultDuration, "load duration")
	flags.IntVar(&cfg.Workers, "workers", defaultWorkers, "maximum concurrent workers")
	flags.DurationVar(&cfg.Timeout, "timeout", defaultTimeout, "per-request timeout")
	flags.IntVar(&cfg.ExpectedStatus, "expected-status", defaultExpectedStatus, "successful HTTP status")
	flags.DurationVar(&cfg.MaxP99, "max-p99", 0, "optional maximum accepted P99 latency (zero disables the gate)")

	if err := flags.Parse(args); err != nil {
		return config{}, errors.New("invalid command-line arguments")
	}
	if flags.NArg() != 0 {
		return config{}, errors.New("positional arguments are not supported")
	}
	if err := validateConfig(cfg); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func validateConfig(cfg config) error {
	if cfg.Method != http.MethodGet && cfg.Method != http.MethodPost {
		return errors.New("method must be GET or POST")
	}
	parsedURL, err := url.ParseRequestURI(cfg.URL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return errors.New("url must be an absolute http or https URL")
	}
	if parsedURL.User != nil {
		return errors.New("url must not contain user information")
	}
	if parsedURL.RawQuery != "" || parsedURL.ForceQuery {
		return errors.New("url must not contain a query")
	}
	if parsedURL.Fragment != "" {
		return errors.New("url must not contain a fragment")
	}
	if cfg.Rate < 1 || cfg.Rate > maxRate {
		return fmt.Errorf("rate must be between 1 and %d", maxRate)
	}
	if cfg.Duration <= 0 || cfg.Duration > maxDuration {
		return fmt.Errorf("duration must be greater than zero and at most %s", maxDuration)
	}
	if cfg.Workers < 1 || cfg.Workers > maxWorkers {
		return fmt.Errorf("workers must be between 1 and %d", maxWorkers)
	}
	if cfg.Timeout <= 0 || cfg.Timeout > maxTimeout {
		return fmt.Errorf("timeout must be greater than zero and at most %s", maxTimeout)
	}
	if cfg.ExpectedStatus < 100 || cfg.ExpectedStatus > 599 {
		return errors.New("expected-status must be between 100 and 599")
	}
	if cfg.MaxP99 < 0 || cfg.MaxP99 > maxTimeout {
		return fmt.Errorf("max-p99 must be zero or at most %s", maxTimeout)
	}

	requests, ok := scheduledRequestCount(cfg.Rate, cfg.Duration)
	if !ok || requests > maxScheduledRequests {
		return fmt.Errorf("rate and duration may schedule at most %d requests", maxScheduledRequests)
	}
	return nil
}

func scheduledRequestCount(rate int, duration time.Duration) (int64, bool) {
	if rate <= 0 || duration <= 0 {
		return 0, false
	}
	seconds := int64(duration / time.Second)
	remainder := duration % time.Second
	if seconds > (int64(maxScheduledRequests)+int64(rate)-1)/int64(rate) {
		return int64(maxScheduledRequests) + 1, true
	}
	whole := seconds * int64(rate)
	fractionNumerator := int64(remainder) * int64(rate)
	fraction := fractionNumerator / int64(time.Second)
	if fractionNumerator%int64(time.Second) != 0 {
		fraction++
	}
	return whole + fraction, true
}

func newHTTPClient(cfg config) *http.Client {
	dialTimeout := cfg.Timeout
	if dialTimeout > 5*time.Second {
		dialTimeout = 5 * time.Second
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           cfg.Workers,
		MaxIdleConnsPerHost:    cfg.Workers,
		MaxConnsPerHost:        cfg.Workers,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    dialTimeout,
		ResponseHeaderTimeout:  cfg.Timeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 1 << 20,
		DisableCompression:     true,
	}
	return &http.Client{Transport: transport, Timeout: cfg.Timeout}
}

func runLoad(ctx context.Context, cfg config, client *http.Client) report {
	started := time.Now()
	deadline := started.Add(cfg.Duration)
	queueSize := cfg.Workers * 4
	jobs := make(chan struct{}, queueSize)
	results := make(chan measurement, cfg.Workers)
	collected := make(chan collectedResults, 1)
	go func() {
		collected <- collectResults(results, cfg.ExpectedStatus)
	}()

	var workers sync.WaitGroup
	startWorkers(ctx, cfg, client, jobs, results, &workers)
	go func() {
		workers.Wait()
		close(results)
	}()

	var scheduled int64
	var dropped int64
	var canceled int64
	for sequence := int64(0); ; sequence++ {
		target := started.Add(scheduleOffset(sequence, cfg.Rate))
		if !target.Before(deadline) {
			break
		}
		if !waitUntil(ctx, target) {
			canceled = 1
			break
		}

		scheduled++
		select {
		case jobs <- struct{}{}:
		default:
			dropped++
		}
	}
	close(jobs)

	result := report{
		Target:         cfg.URL,
		Method:         cfg.Method,
		StartedAt:      started.UTC(),
		Rate:           cfg.Rate,
		DurationMS:     milliseconds(cfg.Duration),
		Workers:        cfg.Workers,
		TimeoutMS:      milliseconds(cfg.Timeout),
		ExpectedStatus: cfg.ExpectedStatus,
		MaxP99MS:       milliseconds(cfg.MaxP99),
		Scheduled:      scheduled,
		Dropped:        dropped,
		Errors:         dropped + canceled,
		StatusCounts:   make(map[string]int64),
	}
	aggregate := <-collected
	client.CloseIdleConnections()
	result.Completed = aggregate.completed
	result.Success = aggregate.success
	result.Errors += aggregate.errors
	result.UnexpectedStatus = aggregate.unexpectedStatus
	result.StatusCounts = aggregate.statusCounts

	finished := time.Now()
	result.FinishedAt = finished.UTC()
	result.ElapsedMS = milliseconds(finished.Sub(started))
	if result.ElapsedMS > 0 {
		result.ActualRPS = float64(result.Completed) / (result.ElapsedMS / 1_000)
	}
	populateLatencyStats(&result, aggregate.latencies)
	result.P99LimitExceeded = cfg.MaxP99 > 0 && result.P99MS > result.MaxP99MS
	return result
}

type collectedResults struct {
	completed        int64
	success          int64
	errors           int64
	unexpectedStatus int64
	statusCounts     map[string]int64
	latencies        []time.Duration
}

func collectResults(results <-chan measurement, expectedStatus int) collectedResults {
	collected := collectedResults{statusCounts: make(map[string]int64)}
	for resultItem := range results {
		collected.completed++
		collected.latencies = append(collected.latencies, resultItem.latency)
		if resultItem.status != 0 {
			collected.statusCounts[strconv.Itoa(resultItem.status)]++
		}
		if resultItem.err != nil {
			collected.errors++
		}
		if resultItem.status != 0 && resultItem.status != expectedStatus {
			collected.unexpectedStatus++
		}
		if resultItem.err == nil && resultItem.status == expectedStatus {
			collected.success++
		}
	}
	return collected
}

func startWorkers(
	ctx context.Context,
	cfg config,
	client *http.Client,
	jobs <-chan struct{},
	results chan<- measurement,
	workers *sync.WaitGroup,
) {
	workers.Add(cfg.Workers)
	for range cfg.Workers {
		go func() {
			defer workers.Done()
			for range jobs {
				results <- measure(ctx, client, cfg.Method, cfg.URL)
			}
		}()
	}
}

func measure(ctx context.Context, client *http.Client, method, target string) measurement {
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return measurement{latency: time.Since(started), err: errors.New("create request")}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)

	response, err := client.Do(request)
	if err != nil {
		return measurement{latency: time.Since(started), err: errors.New("request failed")}
	}
	status := response.StatusCode
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes))
	closeErr := response.Body.Close()
	if readErr != nil {
		return measurement{status: status, latency: time.Since(started), err: errors.New("read response")}
	}
	if closeErr != nil {
		return measurement{status: status, latency: time.Since(started), err: errors.New("close response")}
	}
	return measurement{status: status, latency: time.Since(started)}
}

func scheduleOffset(sequence int64, rate int) time.Duration {
	seconds := sequence / int64(rate)
	remainder := sequence % int64(rate)
	return time.Duration(seconds)*time.Second + time.Duration(remainder)*time.Second/time.Duration(rate)
}

func waitUntil(ctx context.Context, target time.Time) bool {
	delay := time.Until(target)
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func populateLatencyStats(result *report, values []time.Duration) {
	if len(values) == 0 {
		return
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	result.MinMS = milliseconds(sorted[0])
	result.P50MS = milliseconds(percentile(sorted, 0.50))
	result.P95MS = milliseconds(percentile(sorted, 0.95))
	result.P99MS = milliseconds(percentile(sorted, 0.99))
	result.MaxMS = milliseconds(sorted[len(sorted)-1])
}

func percentile(sorted []time.Duration, quantile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if quantile <= 0 {
		return sorted[0]
	}
	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(quantile * float64(len(sorted)))
	if quantile*float64(len(sorted)) == float64(index) {
		index--
	}
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func milliseconds(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func writeJSONLine(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
