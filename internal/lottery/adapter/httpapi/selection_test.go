package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sharedhttp "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpapi"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/randomsource"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/gin-gonic/gin"
)

func TestSelectionHTTPReturnsMaxUint64RewardAsDecimalStrings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	award := selectionHTTPAward(
		t,
		domain.AwardID(math.MaxUint64),
		"Maximum reward",
		domain.Weight(math.MaxUint64),
		domain.AwardOutcomeReward,
	)
	strategy := selectionHTTPStrategy(t, domain.StrategyID(math.MaxUint64), "Maximum strategy", []domain.Award{award})
	var readerContext context.Context
	var readerID domain.StrategyID
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(ctx context.Context, id domain.StrategyID) (domain.Strategy, error) {
			readerContext = ctx
			readerID = id
			return strategy, nil
		}),
		awardSelectorFunc(func(got domain.Strategy) (domain.Award, error) {
			if got.ID() != strategy.ID() {
				t.Fatalf("selector StrategyID = %d, want %d", got.ID(), strategy.ID())
			}
			return award, nil
		}),
	)
	requestID := "selection-max-request"
	timeout := 500 * time.Millisecond
	router := selectionHTTPRouter(t, service, selectionRouterOptions{
		requestID: requestID,
		timeout:   timeout,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, selectionURL(strconvMaxUint64), nil)
	acknowledgeEphemeralSelection(request)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if readerID != domain.StrategyID(math.MaxUint64) {
		t.Fatalf("reader ID = %d, want MaxUint64", readerID)
	}
	if sharedhttp.RequestIDFromContext(readerContext) != requestID {
		t.Fatalf("reader request ID = %q, want %q", sharedhttp.RequestIDFromContext(readerContext), requestID)
	}
	deadline, hasDeadline := readerContext.Deadline()
	if !hasDeadline || time.Until(deadline) > timeout {
		t.Fatalf("reader deadline = %v, present %v; want bounded by %s", deadline, hasDeadline, timeout)
	}
	assertSelectionSuccessHeaders(t, recorder, requestID)
	if location := recorder.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want absent because no resource was created", location)
	}

	var response selectionResponse
	decoder := json.NewDecoder(bytes.NewReader(recorder.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("response contains trailing JSON: %v", err)
	}
	want := selectionResponse{Selection: selectionBody{
		Durability: "ephemeral",
		StrategyID: strconvMaxUint64,
		Award: selectionAward{
			ID:      strconvMaxUint64,
			Name:    "Maximum reward",
			Outcome: string(domain.AwardOutcomeReward),
		},
	}}
	if response != want {
		t.Fatalf("response = %#v, want %#v", response, want)
	}
	for _, forbidden := range []string{"draw_id", "ticket", "random", "location"} {
		if strings.Contains(strings.ToLower(recorder.Body.String()), forbidden) {
			t.Fatalf("response unexpectedly contains %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestSelectionHTTPReturnsNoRewardAsSuccessfulOutcome(t *testing.T) {
	award := selectionHTTPAward(t, 9, "Try again", 7, domain.AwardOutcomeNoReward)
	strategy := selectionHTTPStrategy(t, 77, "No reward strategy", []domain.Award{award})
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
			return strategy, nil
		}),
		awardSelectorFunc(func(domain.Strategy) (domain.Award, error) { return award, nil }),
	)
	router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "no-reward-request"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, selectionURL("77"), nil)
	acknowledgeEphemeralSelection(request)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response selectionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Selection.Award.Outcome != string(domain.AwardOutcomeNoReward) ||
		response.Selection.Award.ID != "9" {
		t.Fatalf("no_reward response = %#v", response)
	}
}

func TestSelectionHTTPRejectsNonCanonicalStrategyIDsBeforeUseCase(t *testing.T) {
	var readerCalls atomic.Int64
	var selectorCalls atomic.Int64
	placeholder := selectionHTTPStrategy(t, 1, "Placeholder", []domain.Award{
		selectionHTTPAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
			readerCalls.Add(1)
			return placeholder, nil
		}),
		awardSelectorFunc(func(domain.Strategy) (domain.Award, error) {
			selectorCalls.Add(1)
			return placeholder.Awards()[0], nil
		}),
	)

	for _, rawID := range []string{
		"0",
		"00",
		"01",
		"+1",
		"-1",
		"%201",
		"1.0",
		"0x1",
		"18446744073709551616",
		"999999999999999999999",
		"not-a-number",
	} {
		t.Run(rawID, func(t *testing.T) {
			router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "invalid-id-request"})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, selectionURL(rawID), nil))
			assertSelectionError(t, recorder, http.StatusBadRequest, "invalid_strategy_id", "invalid-id-request")
		})
	}
	if readerCalls.Load() != 0 || selectorCalls.Load() != 0 {
		t.Fatalf("dependency calls = reader %d, selector %d; want zero", readerCalls.Load(), selectorCalls.Load())
	}
}

func TestSelectionHTTPRejectsAnyBodyAndIdempotencyClaim(t *testing.T) {
	var readerCalls atomic.Int64
	strategy := selectionHTTPStrategy(t, 1, "Placeholder", []domain.Award{
		selectionHTTPAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
			readerCalls.Add(1)
			return strategy, nil
		}),
		awardSelectorFunc(func(domain.Strategy) (domain.Award, error) { return strategy.Awards()[0], nil }),
	)

	t.Run("known length body", func(t *testing.T) {
		router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "body-request"})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, selectionURL("1"), strings.NewReader("{}"))
		acknowledgeEphemeralSelection(request)
		router.ServeHTTP(recorder, request)
		assertSelectionError(t, recorder, http.StatusBadRequest, "request_body_not_allowed", "body-request")
	})

	t.Run("chunked body", func(t *testing.T) {
		router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "chunked-request"})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, selectionURL("1"), strings.NewReader(" "))
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
		acknowledgeEphemeralSelection(request)
		router.ServeHTTP(recorder, request)
		assertSelectionError(t, recorder, http.StatusBadRequest, "request_body_not_allowed", "chunked-request")
	})

	t.Run("empty chunked framing", func(t *testing.T) {
		router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "empty-chunked-request"})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, selectionURL("1"), http.NoBody)
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
		acknowledgeEphemeralSelection(request)
		router.ServeHTTP(recorder, request)
		assertSelectionError(t, recorder, http.StatusBadRequest, "request_body_not_allowed", "empty-chunked-request")
	})

	t.Run("unknown length without transfer encoding", func(t *testing.T) {
		router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "unknown-length-request"})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, selectionURL("1"), http.NoBody)
		request.ContentLength = -1
		acknowledgeEphemeralSelection(request)
		router.ServeHTTP(recorder, request)
		assertSelectionError(t, recorder, http.StatusBadRequest, "request_body_not_allowed", "unknown-length-request")
	})

	for _, value := range []string{"", "client-assumes-replay"} {
		t.Run("idempotency_"+value, func(t *testing.T) {
			router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "idempotency-request"})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, selectionURL("1"), nil)
			request.Header[IdempotencyKeyHeader] = []string{value}
			router.ServeHTTP(recorder, request)
			assertSelectionError(t, recorder, http.StatusBadRequest, "idempotency_not_supported", "idempotency-request")
		})
	}
	if readerCalls.Load() != 0 {
		t.Fatalf("reader calls = %d, want zero", readerCalls.Load())
	}
}

func TestSelectionHTTPRequiresDemoAcknowledgementAndRejectsUndeclaredInput(t *testing.T) {
	var readerCalls atomic.Int64
	strategy := selectionHTTPStrategy(t, 1, "Placeholder", []domain.Award{
		selectionHTTPAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
			readerCalls.Add(1)
			return strategy, nil
		}),
		awardSelectorFunc(func(domain.Strategy) (domain.Award, error) { return strategy.Awards()[0], nil }),
	)

	tests := []struct {
		name      string
		configure func(*http.Request)
		code      string
	}{
		{name: "missing acknowledgement", code: "demo_mode_required"},
		{
			name: "wrong acknowledgement",
			configure: func(request *http.Request) {
				request.Header.Set(DemoModeHeader, "durable-draw")
			},
			code: "demo_mode_required",
		},
		{
			name: "duplicate acknowledgement",
			configure: func(request *http.Request) {
				request.Header[DemoModeHeader] = []string{DemoModeEphemeralSelection, DemoModeEphemeralSelection}
			},
			code: "demo_mode_required",
		},
		{
			name: "query parameter",
			configure: func(request *http.Request) {
				acknowledgeEphemeralSelection(request)
			},
			code: "query_parameters_not_allowed",
		},
		{
			name: "force query",
			configure: func(request *http.Request) {
				acknowledgeEphemeralSelection(request)
				request.URL.ForceQuery = true
			},
			code: "query_parameters_not_allowed",
		},
		{
			name: "declared trailer",
			configure: func(request *http.Request) {
				acknowledgeEphemeralSelection(request)
				request.Trailer = http.Header{"X-Lottery-Ticket": nil}
			},
			code: "request_body_not_allowed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "strict-input-request"})
			target := selectionURL("1")
			if test.name == "query parameter" {
				target += "?seed=caller-controlled"
			}
			request := httptest.NewRequest(http.MethodPost, target, nil)
			if test.configure != nil {
				test.configure(request)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assertSelectionError(t, recorder, http.StatusBadRequest, test.code, "strict-input-request")
		})
	}
	if readerCalls.Load() != 0 {
		t.Fatalf("reader calls = %d, want zero", readerCalls.Load())
	}
}

func TestSelectionHTTPMapsStableErrorClassesWithoutLeakingCauses(t *testing.T) {
	secretCause := errors.New("password=never-expose sql=SELECT secret FROM private")
	strategy := selectionHTTPStrategy(t, 12, "Error strategy", []domain.Award{
		selectionHTTPAward(t, 1, "A", 1, domain.AwardOutcomeReward),
		selectionHTTPAward(t, 2, "B", 1, domain.AwardOutcomeNoReward),
	})

	tests := []struct {
		name        string
		readerErr   error
		selectorErr error
		status      int
		code        string
		logClass    string
	}{
		{name: "not found", readerErr: application.WrapRepositoryError(application.ErrStrategyNotFound, secretCause), status: 404, code: "lottery_strategy_not_found"},
		{name: "retryable", readerErr: application.WrapRepositoryError(application.ErrRepositoryRetryable, secretCause), status: 503, code: "lottery_selection_unavailable", logClass: "repository_retryable"},
		{name: "repository failure", readerErr: application.WrapRepositoryError(application.ErrRepositoryFailure, secretCause), status: 500, code: "internal_error", logClass: "repository_failure"},
		{name: "stored invalid", readerErr: application.WrapRepositoryError(application.ErrStoredStrategyInvalid, secretCause), status: 500, code: "internal_error", logClass: "stored_strategy_invalid"},
		{name: "random source", selectorErr: domain.ErrRandomSourceFailure, status: 503, code: "lottery_selection_unavailable", logClass: "random_source_failure"},
		{name: "source contract", selectorErr: domain.ErrRandomSourceContractViolation, status: 500, code: "internal_error", logClass: "random_source_contract"},
		{name: "selector invariant", selectorErr: domain.ErrSelectionInvariantViolation, status: 500, code: "internal_error", logClass: "selection_invariant"},
		{name: "unknown", selectorErr: secretCause, status: 500, code: "internal_error", logClass: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			service := selectionHTTPService(
				t,
				strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
					return strategy, test.readerErr
				}),
				awardSelectorFunc(func(domain.Strategy) (domain.Award, error) {
					if test.selectorErr != nil {
						return domain.Award{}, test.selectorErr
					}
					return strategy.Awards()[0], nil
				}),
			)
			router := selectionHTTPRouter(t, service, selectionRouterOptions{
				requestID: "mapped-error-request",
				logger:    logger,
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, selectionURL("12"), nil)
			acknowledgeEphemeralSelection(request)
			router.ServeHTTP(recorder, request)
			assertSelectionError(t, recorder, test.status, test.code, "mapped-error-request")
			if strings.Contains(recorder.Body.String(), "never-expose") || strings.Contains(logs.String(), "never-expose") {
				t.Fatalf("secret cause leaked; response=%s logs=%s", recorder.Body.String(), logs.String())
			}
			if test.logClass == "" {
				if strings.Contains(logs.String(), "lottery selection failed") {
					t.Fatalf("expected access log only for client error, got %s", logs.String())
				}
			} else if !strings.Contains(logs.String(), `"error_class":"`+test.logClass+`"`) {
				t.Fatalf("logs = %s, want stable class %q", logs.String(), test.logClass)
			} else if !strings.Contains(logs.String(), `"strategy_id":"12"`) {
				t.Fatalf("logs = %s, want canonical strategy_id", logs.String())
			}
		})
	}
}

func TestSelectionHTTPDeadlineCancelsRepositoryAndReturnsJSONBeforeGatewayBudget(t *testing.T) {
	var readerObservedCancellation atomic.Bool
	var selectorCalls atomic.Int64
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(ctx context.Context, _ domain.StrategyID) (domain.Strategy, error) {
			<-ctx.Done()
			readerObservedCancellation.Store(true)
			return domain.Strategy{}, ctx.Err()
		}),
		awardSelectorFunc(func(domain.Strategy) (domain.Award, error) {
			selectorCalls.Add(1)
			return domain.Award{}, nil
		}),
	)
	router := selectionHTTPRouter(t, service, selectionRouterOptions{
		requestID: "deadline-request",
		timeout:   20 * time.Millisecond,
	})
	recorder := httptest.NewRecorder()
	startedAt := time.Now()
	request := httptest.NewRequest(http.MethodPost, selectionURL("1"), nil)
	acknowledgeEphemeralSelection(request)
	router.ServeHTTP(recorder, request)
	elapsed := time.Since(startedAt)

	assertSelectionError(t, recorder, http.StatusServiceUnavailable, "lottery_selection_unavailable", "deadline-request")
	if !readerObservedCancellation.Load() {
		t.Fatal("repository did not observe the derived deadline")
	}
	if selectorCalls.Load() != 0 {
		t.Fatalf("selector calls = %d, want zero", selectorCalls.Load())
	}
	if elapsed > time.Second {
		t.Fatalf("deadline response took %s, want comfortably below gateway budget", elapsed)
	}
}

func TestSelectionHTTPMethodAndCanonicalPathContracts(t *testing.T) {
	strategy := selectionHTTPStrategy(t, 1, "Path strategy", []domain.Award{
		selectionHTTPAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) { return strategy, nil }),
		awardSelectorFunc(func(domain.Strategy) (domain.Award, error) { return strategy.Awards()[0], nil }),
	)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "method-request"})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(method, selectionURL("1"), nil))
			assertSelectionError(t, recorder, http.StatusMethodNotAllowed, "method_not_allowed", "method-request")
			if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
				t.Fatalf("Allow = %q, want POST", allow)
			}
		})
	}

	router := selectionHTTPRouter(t, service, selectionRouterOptions{requestID: "slash-request"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, selectionURL("1")+"/", nil))
	assertSelectionError(t, recorder, http.StatusNotFound, "route_not_found", "slash-request")
	if recorder.Code == http.StatusTemporaryRedirect || recorder.Code == http.StatusPermanentRedirect {
		t.Fatalf("trailing slash POST redirected with %d", recorder.Code)
	}
}

func TestSelectionHTTPConcurrentRequestsUseSharedCryptoSelectorSafely(t *testing.T) {
	strategy := selectionHTTPStrategy(t, 808, "Concurrent strategy", []domain.Award{
		selectionHTTPAward(t, 1, "Reward", 1, domain.AwardOutcomeReward),
		selectionHTTPAward(t, 2, "No reward", 3, domain.AwardOutcomeNoReward),
	})
	selector, err := domain.NewWeightedSelector(randomsource.NewCryptoSource())
	if err != nil {
		t.Fatalf("construct crypto selector: %v", err)
	}
	var readCalls atomic.Int64
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
			readCalls.Add(1)
			return strategy, nil
		}),
		selector,
	)
	router := selectionHTTPRouter(t, service, selectionRouterOptions{})

	const workers = 100
	var waitGroup sync.WaitGroup
	failures := make(chan string, workers)
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, selectionURL("808"), nil)
			acknowledgeEphemeralSelection(request)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				failures <- recorder.Body.String()
				return
			}
			var response selectionResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				failures <- err.Error()
				return
			}
			if response.Selection.Award.ID != "1" && response.Selection.Award.ID != "2" {
				failures <- recorder.Body.String()
			}
		}()
	}
	waitGroup.Wait()
	close(failures)
	for failure := range failures {
		t.Fatalf("concurrent request failed: %s", failure)
	}
	if readCalls.Load() != workers {
		t.Fatalf("reader calls = %d, want %d", readCalls.Load(), workers)
	}
}

func TestRegisterRoutesRejectsMissingComposition(t *testing.T) {
	if err := RegisterRoutes(nil, &application.EphemeralSelectionService{}, Options{}); !errors.Is(err, ErrRouterRequired) {
		t.Fatalf("nil router error = %v, want router required", err)
	}
	if err := RegisterRoutes(gin.New(), nil, Options{}); !errors.Is(err, ErrServiceRequired) {
		t.Fatalf("nil service error = %v, want service required", err)
	}
	if err := RegisterRoutes(gin.New(), &application.EphemeralSelectionService{}, Options{}); !errors.Is(err, ErrServiceRequired) {
		t.Fatalf("zero service error = %v, want service required", err)
	}
	strategy := selectionHTTPStrategy(t, 1, "Configured", []domain.Award{
		selectionHTTPAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	service := selectionHTTPService(
		t,
		strategyReaderFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) { return strategy, nil }),
		awardSelectorFunc(func(domain.Strategy) (domain.Award, error) { return strategy.Awards()[0], nil }),
	)
	for _, timeout := range []time.Duration{-time.Nanosecond, MaximumSelectionTimeout + time.Nanosecond} {
		if err := RegisterRoutes(gin.New(), service, Options{Timeout: timeout}); !errors.Is(err, ErrTimeoutInvalid) {
			t.Fatalf("timeout %s error = %v, want invalid timeout", timeout, err)
		}
	}
}

const strconvMaxUint64 = "18446744073709551615"

type strategyReaderFunc func(context.Context, domain.StrategyID) (domain.Strategy, error)

func (function strategyReaderFunc) FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error) {
	return function(ctx, id)
}

type awardSelectorFunc func(domain.Strategy) (domain.Award, error)

func (function awardSelectorFunc) Select(strategy domain.Strategy) (domain.Award, error) {
	return function(strategy)
}

type selectionRouterOptions struct {
	requestID string
	timeout   time.Duration
	logger    *slog.Logger
}

func selectionHTTPService(
	t *testing.T,
	reader application.StrategyReader,
	selector application.AwardSelector,
) *application.EphemeralSelectionService {
	t.Helper()
	service, err := application.NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct selection service: %v", err)
	}
	return service
}

func selectionHTTPRouter(
	t *testing.T,
	service *application.EphemeralSelectionService,
	options selectionRouterOptions,
) *gin.Engine {
	t.Helper()
	requestID := options.requestID
	if requestID == "" {
		requestID = "selection-test-request"
	}
	logger := options.logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	}
	router := sharedhttp.NewRouter(sharedhttp.RouterOptions{
		Logger:             logger,
		RequestIDGenerator: func() string { return requestID },
	})
	if err := RegisterRoutes(router, service, Options{Logger: logger, Timeout: options.timeout}); err != nil {
		t.Fatalf("register Lottery routes: %v", err)
	}
	return router
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func selectionURL(strategyID string) string {
	return strings.Replace(SelectionPath, ":"+StrategyIDParameter, strategyID, 1)
}

func acknowledgeEphemeralSelection(request *http.Request) {
	request.Header.Set(DemoModeHeader, DemoModeEphemeralSelection)
}

func selectionHTTPStrategy(
	t *testing.T,
	id domain.StrategyID,
	name string,
	awards []domain.Award,
) domain.Strategy {
	t.Helper()
	strategy, err := domain.NewStrategy(id, name, awards)
	if err != nil {
		t.Fatalf("construct Strategy: %v", err)
	}
	return strategy
}

func selectionHTTPAward(
	t *testing.T,
	id domain.AwardID,
	name string,
	weight domain.Weight,
	outcome domain.AwardOutcome,
) domain.Award {
	t.Helper()
	award, err := domain.NewAward(id, name, weight, outcome)
	if err != nil {
		t.Fatalf("construct Award: %v", err)
	}
	return award
}

func assertSelectionSuccessHeaders(t *testing.T, recorder *httptest.ResponseRecorder, requestID string) {
	t.Helper()
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", contentType)
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
	if got := recorder.Header().Get(sharedhttp.RequestIDHeader); got != requestID {
		t.Fatalf("request ID header = %q, want %q", got, requestID)
	}
}

func assertSelectionError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	status int,
	code string,
	requestID string,
) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, status, recorder.Body.String())
	}
	assertSelectionSuccessHeaders(t, recorder, requestID)
	var response struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != code || response.Error.Message == "" || response.Error.RequestID != requestID {
		t.Fatalf("error response = %#v, want code %q and request ID %q", response.Error, code, requestID)
	}
}
