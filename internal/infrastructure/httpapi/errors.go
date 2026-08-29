package httpapi

import (
	"net/http"

	"github.com/Atingaii/GrowthOS-Go/internal/platform/fault"
	"github.com/gin-gonic/gin"
)

var (
	routeNotFoundFault = fault.MustNew(
		fault.KindNotFound,
		"route_not_found",
		"resource not found",
		nil,
	)
	methodNotAllowedFault = fault.MustNew(
		fault.KindInvalid,
		"method_not_allowed",
		"method not allowed",
		nil,
	)
	internalServerFault = fault.MustNew(
		fault.KindInternal,
		"internal_error",
		"internal server error",
		nil,
	)
	dependencyUnavailableFault = fault.MustNew(
		fault.KindUnavailable,
		"dependency_unavailable",
		"service unavailable",
		nil,
	)
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// AbortWithError maps a transport-independent fault to its HTTP status and
// writes the stable public error envelope. Unknown errors are intentionally
// reduced to a generic internal error.
func AbortWithError(ginContext *gin.Context, err error) {
	status, publicFault := httpFault(err)
	abortWithFaultStatus(ginContext, status, publicFault)
}

func httpFault(err error) (int, *fault.Error) {
	publicFault, ok := fault.As(err)
	if !ok {
		return http.StatusInternalServerError, internalServerFault
	}
	return statusForKind(publicFault.Kind()), publicFault
}

func statusForKind(kind fault.Kind) int {
	switch kind {
	case fault.KindInvalid:
		return http.StatusBadRequest
	case fault.KindUnauthenticated:
		return http.StatusUnauthorized
	case fault.KindForbidden:
		return http.StatusForbidden
	case fault.KindNotFound:
		return http.StatusNotFound
	case fault.KindConflict:
		return http.StatusConflict
	case fault.KindRateLimited:
		return http.StatusTooManyRequests
	case fault.KindUnavailable:
		return http.StatusServiceUnavailable
	case fault.KindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func abortWithFaultStatus(ginContext *gin.Context, status int, publicFault *fault.Error) {
	if publicFault == nil {
		publicFault = internalServerFault
	}
	ginContext.AbortWithStatusJSON(status, errorEnvelope{
		Error: errorBody{
			Code:      publicFault.Code(),
			Message:   publicFault.PublicMessage(),
			RequestID: RequestID(ginContext),
		},
	})
}
