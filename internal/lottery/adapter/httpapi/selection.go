package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	sharedhttp "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpapi"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/fault"
	"github.com/gin-gonic/gin"
)

const (
	// SelectionPath is versioned because it is a business contract. One POST
	// performs a new ephemeral selection and never creates a durable DrawResult.
	SelectionPath = "/api/v1/lottery/strategies/:strategy_id/selections"
	// StrategyIDParameter is the Gin path parameter used by SelectionPath.
	StrategyIDParameter = "strategy_id"
	// IdempotencyKeyHeader is rejected explicitly because this endpoint has no
	// persisted result to which a retry key could be bound.
	IdempotencyKeyHeader = "Idempotency-Key"
	// DefaultSelectionTimeout stays below the Compose gateway's four-second
	// upstream budget. Deployments should still pass their validated setting.
	DefaultSelectionTimeout = 3 * time.Second
)

var (
	ErrRouterRequired  = errors.New("lottery HTTP adapter: router is required")
	ErrServiceRequired = errors.New("lottery HTTP adapter: selection service is required")

	invalidStrategyIDFault = fault.MustNew(
		fault.KindInvalid,
		"invalid_strategy_id",
		"strategy_id must be a canonical decimal integer from 1 through 18446744073709551615",
		nil,
	)
	requestBodyNotAllowedFault = fault.MustNew(
		fault.KindInvalid,
		"request_body_not_allowed",
		"request body is not allowed",
		nil,
	)
	idempotencyNotSupportedFault = fault.MustNew(
		fault.KindInvalid,
		"idempotency_not_supported",
		"idempotency is not supported for ephemeral selections",
		nil,
	)
	strategyNotFoundFault = fault.MustNew(
		fault.KindNotFound,
		"lottery_strategy_not_found",
		"lottery strategy not found",
		nil,
	)
	selectionUnavailableFault = fault.MustNew(
		fault.KindUnavailable,
		"lottery_selection_unavailable",
		"lottery selection is temporarily unavailable",
		nil,
	)
)

// Options contains bounded operational dependencies for the Lottery handler.
type Options struct {
	Logger  *slog.Logger
	Timeout time.Duration
}

// RegisterRoutes mounts the first Lottery business contract on a shared Gin
// engine. Missing composition dependencies fail startup instead of silently
// omitting the route or deferring failure until a request arrives.
func RegisterRoutes(
	router *gin.Engine,
	service *application.EphemeralSelectionService,
	options Options,
) error {
	if router == nil {
		return ErrRouterRequired
	}
	if service == nil {
		return ErrServiceRequired
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultSelectionTimeout
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	router.POST(SelectionPath, newSelectionHandler(service, logger, timeout))
	return nil
}

type selectionResponse struct {
	Selection selectionBody `json:"selection"`
}

type selectionBody struct {
	Durability string            `json:"durability"`
	Strategy   selectionStrategy `json:"strategy"`
	Award      selectionAward    `json:"award"`
}

type selectionStrategy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	TotalWeight string `json:"total_weight"`
}

type selectionAward struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Weight  string `json:"weight"`
	Outcome string `json:"outcome"`
}

func newSelectionHandler(
	service *application.EphemeralSelectionService,
	logger *slog.Logger,
	timeout time.Duration,
) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		strategyID, err := parseStrategyID(ginContext.Param(StrategyIDParameter))
		if err != nil {
			sharedhttp.AbortWithError(ginContext, invalidStrategyIDFault)
			return
		}
		if len(ginContext.Request.Header.Values(IdempotencyKeyHeader)) != 0 {
			sharedhttp.AbortWithError(ginContext, idempotencyNotSupportedFault)
			return
		}
		if !emptyRequestBody(ginContext) {
			sharedhttp.AbortWithError(ginContext, requestBodyNotAllowedFault)
			return
		}

		selectionContext, cancel := context.WithTimeout(ginContext.Request.Context(), timeout)
		defer cancel()
		selection, err := service.Select(selectionContext, strategyID)
		if err == nil {
			err = selectionContext.Err()
		}
		if err != nil {
			publicFault, class := publicSelectionFault(err)
			logSelectionFailure(logger, selectionContext, ginContext, publicFault, class)
			sharedhttp.AbortWithError(ginContext, publicFault)
			return
		}

		ginContext.Header("Cache-Control", "no-store")
		ginContext.JSON(http.StatusOK, mapSelection(selection))
	}
}

func parseStrategyID(raw string) (domain.StrategyID, error) {
	if raw == "" || len(raw) > 20 {
		return 0, domain.ErrStrategyIDRequired
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed == 0 || strconv.FormatUint(parsed, 10) != raw {
		return 0, domain.ErrStrategyIDRequired
	}
	return domain.StrategyID(parsed), nil
}

func emptyRequestBody(ginContext *gin.Context) bool {
	request := ginContext.Request
	if request == nil || request.Body == nil || request.Body == http.NoBody {
		return true
	}
	if request.ContentLength > 0 {
		return false
	}
	limitedBody := http.MaxBytesReader(ginContext.Writer, request.Body, 0)
	_, err := io.Copy(io.Discard, limitedBody)
	return err == nil
}

func publicSelectionFault(err error) (*fault.Error, string) {
	switch {
	case errors.Is(err, application.ErrStrategyNotFound):
		return strategyNotFoundFault, "strategy_not_found"
	case errors.Is(err, context.Canceled):
		return selectionUnavailableFault, "request_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return selectionUnavailableFault, "request_deadline"
	case errors.Is(err, application.ErrRepositoryRetryable):
		return selectionUnavailableFault, "repository_retryable"
	case errors.Is(err, domain.ErrRandomSourceFailure):
		return selectionUnavailableFault, "random_source_failure"
	case errors.Is(err, application.ErrRepositoryFailure):
		return sharedInternalFault(), "repository_failure"
	case errors.Is(err, application.ErrRepositoryNotConfigured):
		return sharedInternalFault(), "repository_not_configured"
	case errors.Is(err, application.ErrStoredStrategyInvalid):
		return sharedInternalFault(), "stored_strategy_invalid"
	case errors.Is(err, application.ErrSelectionInvalidArgument):
		return sharedInternalFault(), "use_case_invalid_argument"
	case errors.Is(err, application.ErrSelectionNotConfigured):
		return sharedInternalFault(), "use_case_not_configured"
	case errors.Is(err, application.ErrSelectionResultInvalid):
		return sharedInternalFault(), "selection_result_invalid"
	case errors.Is(err, domain.ErrSelectorNotConfigured):
		return sharedInternalFault(), "selector_not_configured"
	case errors.Is(err, domain.ErrSelectionStrategyInvalid):
		return sharedInternalFault(), "selection_strategy_invalid"
	case errors.Is(err, domain.ErrRandomSourceContractViolation):
		return sharedInternalFault(), "random_source_contract"
	case errors.Is(err, domain.ErrSelectionInvariantViolation):
		return sharedInternalFault(), "selection_invariant"
	default:
		return sharedInternalFault(), "unknown"
	}
}

func sharedInternalFault() *fault.Error {
	return fault.MustNew(fault.KindInternal, "internal_error", "internal server error", nil)
}

func logSelectionFailure(
	logger *slog.Logger,
	ctx context.Context,
	ginContext *gin.Context,
	publicFault *fault.Error,
	class string,
) {
	attributes := []any{
		slog.String("request_id", sharedhttp.RequestID(ginContext)),
		slog.String("error_class", class),
	}
	if publicFault.Kind() == fault.KindUnavailable {
		logger.WarnContext(ctx, "lottery selection failed", attributes...)
		return
	}
	if publicFault.Kind() == fault.KindInternal {
		logger.ErrorContext(ctx, "lottery selection failed", attributes...)
	}
}

func mapSelection(selection application.EphemeralSelection) selectionResponse {
	strategy := selection.Strategy
	award := selection.Award
	return selectionResponse{Selection: selectionBody{
		Durability: "ephemeral",
		Strategy: selectionStrategy{
			ID:          strconv.FormatUint(uint64(strategy.ID()), 10),
			Name:        strategy.Name(),
			TotalWeight: strconv.FormatUint(strategy.TotalWeight(), 10),
		},
		Award: selectionAward{
			ID:      strconv.FormatUint(uint64(award.ID()), 10),
			Name:    award.Name(),
			Weight:  strconv.FormatUint(uint64(award.Weight()), 10),
			Outcome: string(award.Outcome()),
		},
	}}
}
