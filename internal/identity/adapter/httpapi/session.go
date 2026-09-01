package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"reflect"
	"time"
	"unicode/utf8"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	sharedhttp "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpapi"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/fault"
	"github.com/gin-gonic/gin"
)

const (
	SessionPath           = "/api/v1/session"
	CSRFHeader            = "X-CSRF-Token"
	MaximumLoginBodyBytes = 2048
	DefaultHandlerTimeout = 3 * time.Second
	MaximumHandlerTimeout = 30 * time.Second
)

var (
	ErrRouterRequired       = errors.New("identity HTTP adapter: router is required")
	ErrLoginRequired        = errors.New("identity HTTP adapter: login service is required")
	ErrResolveRequired      = errors.New("identity HTTP adapter: resolve service is required")
	ErrRevokeRequired       = errors.New("identity HTTP adapter: revoke service is required")
	ErrCookiePolicyRequired = errors.New("identity HTTP adapter: cookie policy is required")
	ErrCSRFRequired         = errors.New("identity HTTP adapter: csrf protector is required")
	ErrRequestGuardRequired = errors.New("identity HTTP adapter: request guard is required")
	ErrDigesterRequired     = errors.New("identity HTTP adapter: throttle digester is required")
	ErrClockRequired        = errors.New("identity HTTP adapter: clock is required")
	ErrOriginMismatch       = errors.New("identity HTTP adapter: cookie and request origins differ")
	ErrTimeoutInvalid       = errors.New("identity HTTP adapter: handler timeout is invalid")
)

var (
	invalidRequestFault = fault.MustNew(
		fault.KindInvalid, "invalid_request", "invalid request", nil,
	)
	unsupportedMediaTypeFault = fault.MustNew(
		fault.KindInvalid, "unsupported_media_type", "unsupported media type", nil,
	)
	authenticationFailedFault = fault.MustNew(
		fault.KindUnauthenticated, "authentication_failed", "authentication failed", nil,
	)
	unauthenticatedFault = fault.MustNew(
		fault.KindUnauthenticated, "unauthenticated", "authentication required", nil,
	)
	requestOriginRejectedFault = fault.MustNew(
		fault.KindForbidden, "request_origin_rejected", "request origin rejected", nil,
	)
	authenticationThrottledFault = fault.MustNew(
		fault.KindRateLimited, "authentication_throttled", "authentication throttled", nil,
	)
	authenticationUnavailableFault = fault.MustNew(
		fault.KindUnavailable, "authentication_unavailable", "authentication temporarily unavailable", nil,
	)
	revocationIndeterminateFault = fault.MustNew(
		fault.KindUnavailable,
		"session_revocation_indeterminate",
		"session revocation could not be confirmed",
		nil,
	)
	internalErrorFault = fault.MustNew(
		fault.KindInternal, "internal_error", "internal server error", nil,
	)
)

// LoginUseCase is the narrow application boundary required by the transport.
type LoginUseCase interface {
	Validate() error
	Login(context.Context, identityapp.LoginCommand) (identityapp.IssuedSession, error)
}

// ResolveUseCase is the narrow current-session application boundary.
type ResolveUseCase interface {
	Validate() error
	Resolve(context.Context, []byte) (identityapp.VerifiedSession, error)
}

// RevokeUseCase is the narrow current-session revocation boundary.
type RevokeUseCase interface {
	Validate() error
	RevokeCurrent(context.Context, []byte) error
}

// CookiePolicy owns the exact environment-specific bearer Cookie grammar.
type CookiePolicy interface {
	Validate() error
	Name() string
	PublicOrigin() string
	Read(*http.Request) ([]byte, error)
	ReadOptional(*http.Request) ([]byte, bool, error)
	Build([]byte, time.Time, time.Time) (*http.Cookie, error)
	Clear() (*http.Cookie, error)
}

// CSRFProtector issues and verifies tokens bound to one session digest.
type CSRFProtector interface {
	Issue(identity.TokenDigest) (string, error)
	Verify(string, identity.TokenDigest, time.Time) error
}

// RequestGuard owns exact Origin, Fetch Metadata, and connected-peer parsing.
type RequestGuard interface {
	Validate() error
	PublicOrigin() string
	ValidateUnsafe(*http.Request) error
	TrustedSource(*http.Request) (netip.Addr, error)
}

// ThrottleDigester turns bounded canonical login/source values into opaque keys.
type ThrottleDigester interface {
	DigestLogin(identity.LoginName) (identity.ThrottleDigest, error)
	DigestSource(netip.Addr) (identity.ThrottleDigest, error)
}

// Clock supplies the CSRF verification and Cookie construction instant.
type Clock interface {
	Now() time.Time
}

type Dependencies struct {
	Login    LoginUseCase
	Resolve  ResolveUseCase
	Revoke   RevokeUseCase
	Cookies  CookiePolicy
	CSRF     CSRFProtector
	Guard    RequestGuard
	Digester ThrottleDigester
	Clock    Clock
}

type Options struct {
	Logger  *slog.Logger
	Timeout time.Duration
}

// RegisterRoutes mounts exactly POST, GET, and DELETE /api/v1/session.
func RegisterRoutes(router *gin.Engine, dependencies Dependencies, options Options) error {
	if router == nil {
		return ErrRouterRequired
	}
	if dependencyIsNil(dependencies.Login) || dependencies.Login.Validate() != nil {
		return ErrLoginRequired
	}
	if dependencyIsNil(dependencies.Resolve) || dependencies.Resolve.Validate() != nil {
		return ErrResolveRequired
	}
	if dependencyIsNil(dependencies.Revoke) || dependencies.Revoke.Validate() != nil {
		return ErrRevokeRequired
	}
	if dependencyIsNil(dependencies.Cookies) || dependencies.Cookies.Validate() != nil {
		return ErrCookiePolicyRequired
	}
	if dependencyIsNil(dependencies.CSRF) {
		return ErrCSRFRequired
	}
	if dependencyIsNil(dependencies.Guard) || dependencies.Guard.Validate() != nil {
		return ErrRequestGuardRequired
	}
	if dependencyIsNil(dependencies.Digester) {
		return ErrDigesterRequired
	}
	if dependencyIsNil(dependencies.Clock) {
		return ErrClockRequired
	}
	if dependencies.Cookies.PublicOrigin() == "" ||
		dependencies.Cookies.PublicOrigin() != dependencies.Guard.PublicOrigin() {
		return ErrOriginMismatch
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = DefaultHandlerTimeout
	}
	if timeout < 0 || timeout > MaximumHandlerTimeout {
		return ErrTimeoutInvalid
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	handler := &sessionHandler{dependencies: dependencies, timeout: timeout, logger: logger}
	router.POST(SessionPath, handler.login)
	router.GET(SessionPath, handler.current)
	router.DELETE(SessionPath, handler.logout)
	return nil
}

type sessionHandler struct {
	dependencies Dependencies
	timeout      time.Duration
	logger       *slog.Logger
}

type loginRequest struct {
	loginName string
	password  []byte
}

type sessionEnvelope struct {
	Data sessionData `json:"data"`
}

type sessionData struct {
	Authenticated     bool          `json:"authenticated"`
	Principal         principalData `json:"principal"`
	IdleExpiresAt     string        `json:"idle_expires_at"`
	AbsoluteExpiresAt string        `json:"absolute_expires_at"`
	CSRFToken         string        `json:"csrf_token"`
}

type principalData struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func (handler *sessionHandler) login(ginContext *gin.Context) {
	ctx, cancel := handler.begin(ginContext)
	defer cancel()
	request := ginContext.Request
	if requestHasQuery(request) || hasForbiddenCredentialSource(request) {
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "login", "request_shape")
		return
	}
	if !exactJSONMediaType(request) {
		handler.abort(ginContext, http.StatusUnsupportedMediaType, unsupportedMediaTypeFault, "login", "media_type")
		return
	}
	if handler.dependencies.Guard.ValidateUnsafe(request) != nil {
		handler.abort(ginContext, http.StatusForbidden, requestOriginRejectedFault, "login", "origin")
		return
	}
	source, err := handler.dependencies.Guard.TrustedSource(request)
	if err != nil {
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "login", "source")
		return
	}
	input, err := decodeLoginRequest(request)
	if err != nil {
		if ctx.Err() != nil {
			handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "login", "request_deadline")
			return
		}
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "login", "body")
		return
	}
	defer clear(input.password)
	loginName, err := identity.NewLoginName(input.loginName)
	if err != nil {
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "login", "login_name")
		return
	}
	loginDigest, err := handler.dependencies.Digester.DigestLogin(loginName)
	if err != nil {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "login", "login_digest")
		return
	}
	sourceDigest, err := handler.dependencies.Digester.DigestSource(source)
	if err != nil {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "login", "source_digest")
		return
	}
	previousToken, err := handler.readOptionalLoginCookie(request)
	if err != nil {
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "login", "cookie")
		return
	}
	defer clear(previousToken)
	command, err := identityapp.NewLoginCommand(
		loginName, input.password, loginDigest, sourceDigest, previousToken,
	)
	if err != nil {
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "login", "command")
		return
	}
	issued, err := handler.dependencies.Login.Login(ctx, command)
	if err != nil {
		handler.abortApplication(ginContext, err, "login")
		return
	}
	if issued.Validate() != nil {
		handler.abort(ginContext, http.StatusInternalServerError, internalErrorFault, "login", "invalid_output")
		return
	}
	rawToken := issued.RawToken()
	defer clear(rawToken)
	verified := issued.VerifiedSession()
	csrfToken, err := handler.issueCSRF(rawToken)
	if err != nil || csrfToken == "" {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "login", "csrf_issue")
		return
	}
	now := canonicalInstant(handler.dependencies.Clock.Now())
	cookie, err := handler.dependencies.Cookies.Build(rawToken, now, verified.AbsoluteExpiresAt())
	if err != nil || cookie == nil {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "login", "cookie_build")
		return
	}
	if ctx.Err() != nil {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "login", "request_deadline")
		return
	}
	http.SetCookie(ginContext.Writer, cookie)
	handler.writeSession(ginContext, http.StatusCreated, verified, csrfToken)
}

func (handler *sessionHandler) current(ginContext *gin.Context) {
	ctx, cancel := handler.begin(ginContext)
	defer cancel()
	request := ginContext.Request
	if requestHasQuery(request) || !emptyBodyFraming(request) || hasForbiddenCredentialSource(request) {
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "current", "request_shape")
		return
	}
	rawToken, err := handler.dependencies.Cookies.Read(request)
	if err != nil {
		if !handler.clearCookie(ginContext, "current") {
			return
		}
		handler.abort(ginContext, http.StatusUnauthorized, unauthenticatedFault, "current", "cookie")
		return
	}
	defer clear(rawToken)
	csrfToken, err := handler.issueCSRF(rawToken)
	if err != nil || csrfToken == "" {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "current", "csrf_issue")
		return
	}
	verified, err := handler.dependencies.Resolve.Resolve(ctx, rawToken)
	if err != nil {
		if errors.Is(err, identityapp.ErrUnauthenticated) {
			if !handler.clearCookie(ginContext, "current") {
				return
			}
		}
		handler.abortApplication(ginContext, err, "current")
		return
	}
	if verified.Validate() != nil {
		handler.abort(ginContext, http.StatusInternalServerError, internalErrorFault, "current", "invalid_output")
		return
	}
	if ctx.Err() != nil {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "current", "request_deadline")
		return
	}
	handler.writeSession(ginContext, http.StatusOK, verified, csrfToken)
}

func (handler *sessionHandler) logout(ginContext *gin.Context) {
	ctx, cancel := handler.begin(ginContext)
	defer cancel()
	request := ginContext.Request
	if requestHasQuery(request) || !emptyBodyFraming(request) || hasForbiddenCredentialSource(request) {
		handler.abort(ginContext, http.StatusBadRequest, invalidRequestFault, "logout", "request_shape")
		return
	}
	if handler.dependencies.Guard.ValidateUnsafe(request) != nil {
		handler.abort(ginContext, http.StatusForbidden, requestOriginRejectedFault, "logout", "origin")
		return
	}
	rawToken, err := handler.dependencies.Cookies.Read(request)
	if err != nil {
		if !handler.clearCookie(ginContext, "logout") {
			return
		}
		handler.abort(ginContext, http.StatusUnauthorized, unauthenticatedFault, "logout", "cookie")
		return
	}
	defer clear(rawToken)
	csrfValues := request.Header.Values(CSRFHeader)
	if len(csrfValues) != 1 || csrfValues[0] == "" {
		handler.abort(ginContext, http.StatusForbidden, requestOriginRejectedFault, "logout", "csrf")
		return
	}
	now := canonicalInstant(handler.dependencies.Clock.Now())
	if now.IsZero() {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "logout", "clock")
		return
	}
	digest, err := tokenDigest(rawToken)
	if err != nil || handler.dependencies.CSRF.Verify(csrfValues[0], digest, now) != nil {
		handler.abort(ginContext, http.StatusForbidden, requestOriginRejectedFault, "logout", "csrf")
		return
	}
	if err := handler.dependencies.Revoke.RevokeCurrent(ctx, rawToken); err != nil {
		if errors.Is(err, identityapp.ErrUnauthenticated) ||
			errors.Is(err, identityapp.ErrRevocationIndeterminate) {
			if !handler.clearCookie(ginContext, "logout") {
				return
			}
		}
		handler.abortApplication(ginContext, err, "logout")
		return
	}
	if !handler.clearCookie(ginContext, "logout") {
		return
	}
	if ctx.Err() != nil {
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, "logout", "request_deadline")
		return
	}
	ginContext.Status(http.StatusNoContent)
}

func (handler *sessionHandler) begin(ginContext *gin.Context) (context.Context, context.CancelFunc) {
	setSafetyHeaders(ginContext.Writer.Header())
	ctx, cancel := context.WithTimeout(ginContext.Request.Context(), handler.timeout)
	ginContext.Request = ginContext.Request.WithContext(ctx)
	return ctx, cancel
}

func (handler *sessionHandler) issueCSRF(rawToken []byte) (string, error) {
	digest, err := tokenDigest(rawToken)
	if err != nil {
		return "", err
	}
	return handler.dependencies.CSRF.Issue(digest)
}

func (handler *sessionHandler) writeSession(
	ginContext *gin.Context,
	status int,
	verified identityapp.VerifiedSession,
	csrfToken string,
) {
	ginContext.JSON(status, sessionEnvelope{Data: sessionData{
		Authenticated: true,
		Principal: principalData{
			Kind: "human",
			ID:   verified.PrincipalID().String(),
		},
		IdleExpiresAt:     verified.IdleExpiresAt().Format(time.RFC3339Nano),
		AbsoluteExpiresAt: verified.AbsoluteExpiresAt().Format(time.RFC3339Nano),
		CSRFToken:         csrfToken,
	}})
}

func (handler *sessionHandler) readOptionalLoginCookie(request *http.Request) ([]byte, error) {
	rawToken, present, err := handler.dependencies.Cookies.ReadOptional(request)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	return rawToken, nil
}

func (handler *sessionHandler) clearCookie(ginContext *gin.Context, operation string) bool {
	cookie, err := handler.dependencies.Cookies.Clear()
	if err != nil || cookie == nil {
		handler.abort(
			ginContext,
			http.StatusServiceUnavailable,
			authenticationUnavailableFault,
			operation,
			"cookie_clear",
		)
		return false
	}
	http.SetCookie(ginContext.Writer, cookie)
	return true
}

func (handler *sessionHandler) abortApplication(
	ginContext *gin.Context,
	err error,
	operation string,
) {
	switch {
	case errors.Is(err, identityapp.ErrAuthenticationFailed):
		handler.abort(ginContext, http.StatusUnauthorized, authenticationFailedFault, operation, "authentication_failed")
	case errors.Is(err, identityapp.ErrUnauthenticated):
		handler.abort(ginContext, http.StatusUnauthorized, unauthenticatedFault, operation, "unauthenticated")
	case errors.Is(err, identityapp.ErrAuthenticationThrottled):
		handler.abort(ginContext, http.StatusTooManyRequests, authenticationThrottledFault, operation, "throttled")
	case errors.Is(err, identityapp.ErrRevocationIndeterminate):
		handler.abort(ginContext, http.StatusServiceUnavailable, revocationIndeterminateFault, operation, "revocation_indeterminate")
	case errors.Is(err, identityapp.ErrAuthenticationUnavailable),
		errors.Is(err, identityapp.ErrCommitOutcomeUnknown),
		errors.Is(err, identityapp.ErrOperationCanceled),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		handler.abort(ginContext, http.StatusServiceUnavailable, authenticationUnavailableFault, operation, "unavailable")
	default:
		handler.abort(ginContext, http.StatusInternalServerError, internalErrorFault, operation, "internal")
	}
}

func (handler *sessionHandler) abort(
	ginContext *gin.Context,
	status int,
	publicFault *fault.Error,
	operation string,
	class string,
) {
	if publicFault == nil {
		publicFault = internalErrorFault
		status = http.StatusInternalServerError
	}
	handler.logger.WarnContext(
		ginContext.Request.Context(),
		"identity session request failed",
		slog.String("operation", operation),
		slog.String("result_class", class),
		slog.String("request_id", sharedhttp.RequestID(ginContext)),
	)
	ginContext.AbortWithStatusJSON(status, errorEnvelope{Error: errorBody{
		Code:      publicFault.Code(),
		Message:   publicFault.PublicMessage(),
		RequestID: sharedhttp.RequestID(ginContext),
	}})
}

func decodeLoginRequest(request *http.Request) (loginRequest, error) {
	if request == nil || request.Body == nil || request.Body == http.NoBody ||
		request.ContentLength <= 0 || request.ContentLength > MaximumLoginBodyBytes ||
		len(request.TransferEncoding) != 0 || len(request.Trailer) != 0 ||
		request.Context() == nil || request.Context().Err() != nil {
		return loginRequest{}, errors.New("invalid login body")
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaximumLoginBodyBytes+1))
	defer clear(body)
	if err != nil || request.Context().Err() != nil || len(body) == 0 ||
		len(body) > MaximumLoginBodyBytes || int64(len(body)) != request.ContentLength ||
		!utf8.Valid(body) || !validUnicodeEscapes(body) {
		return loginRequest{}, errors.New("invalid login body")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return loginRequest{}, errors.New("login body must be an object")
	}
	seen := make(map[string]struct{}, 2)
	var result loginRequest
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			clear(result.password)
			return loginRequest{}, err
		}
		field, ok := token.(string)
		if !ok {
			clear(result.password)
			return loginRequest{}, errors.New("invalid field")
		}
		if _, duplicate := seen[field]; duplicate {
			clear(result.password)
			return loginRequest{}, errors.New("duplicate field")
		}
		seen[field] = struct{}{}
		var value string
		if err := decoder.Decode(&value); err != nil {
			clear(result.password)
			return loginRequest{}, err
		}
		switch field {
		case "login_name":
			result.loginName = value
		case "password":
			result.password = []byte(value)
		default:
			clear(result.password)
			return loginRequest{}, errors.New("unknown field")
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		clear(result.password)
		return loginRequest{}, errors.New("invalid object")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		clear(result.password)
		return loginRequest{}, errors.New("trailing data")
	}
	if len(seen) != 2 {
		clear(result.password)
		return loginRequest{}, errors.New("missing field")
	}
	return result, nil
}

// validUnicodeEscapes closes encoding/json's documented replacement of
// unpaired UTF-16 surrogates. Password bytes must not be silently rewritten.
func validUnicodeEscapes(body []byte) bool {
	inString := false
	for index := 0; index < len(body); index++ {
		switch body[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(body) {
				continue
			}
			if body[index+1] != 'u' {
				index++
				continue
			}
			value, ok := decodeHexQuad(body, index+2)
			if !ok {
				return false
			}
			if value >= 0xdc00 && value <= 0xdfff {
				return false
			}
			if value >= 0xd800 && value <= 0xdbff {
				if index+11 >= len(body) || body[index+6] != '\\' || body[index+7] != 'u' {
					return false
				}
				low, validLow := decodeHexQuad(body, index+8)
				if !validLow || low < 0xdc00 || low > 0xdfff {
					return false
				}
				index += 11
				continue
			}
			index += 5
		}
	}
	return true
}

func decodeHexQuad(body []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(body) {
		return 0, false
	}
	var value uint16
	for index := start; index < start+4; index++ {
		value <<= 4
		switch character := body[index]; {
		case character >= '0' && character <= '9':
			value |= uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value |= uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value |= uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func exactJSONMediaType(request *http.Request) bool {
	if request == nil {
		return false
	}
	values := request.Header.Values("Content-Type")
	return len(values) == 1 && values[0] == "application/json"
}

func requestHasQuery(request *http.Request) bool {
	return request != nil && request.URL != nil &&
		(request.URL.RawQuery != "" || request.URL.ForceQuery)
}

func emptyBodyFraming(request *http.Request) bool {
	return request != nil && request.ContentLength == 0 &&
		len(request.TransferEncoding) == 0 && len(request.Trailer) == 0 &&
		(request.Body == nil || request.Body == http.NoBody)
}

var forbiddenCredentialHeaders = [...]string{
	"Authorization",
	"X-Account-ID",
	"X-Principal-ID",
	"X-Role",
	"X-Permission",
	"X-Scope",
	"X-Tenant-ID",
}

func hasForbiddenCredentialSource(request *http.Request) bool {
	if request == nil {
		return true
	}
	for _, name := range forbiddenCredentialHeaders {
		if len(request.Header.Values(name)) != 0 {
			return true
		}
	}
	return false
}

func tokenDigest(rawToken []byte) (identity.TokenDigest, error) {
	if len(rawToken) != identityapp.SessionTokenBytes {
		return identity.TokenDigest{}, errors.New("invalid session token")
	}
	value := sha256.Sum256(rawToken)
	return identity.NewTokenDigest(value[:])
}

func canonicalInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0).Truncate(time.Microsecond)
}

func setSafetyHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

func dependencyIsNil(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer,
		reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
