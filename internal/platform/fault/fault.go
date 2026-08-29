// Package fault defines transport-independent application failures.
package fault

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxCodeLength          = 64
	maxPublicMessageLength = 256
)

// Kind classifies a failure without coupling application code to a transport.
type Kind string

const (
	KindInvalid         Kind = "invalid"
	KindUnauthenticated Kind = "unauthenticated"
	KindForbidden       Kind = "forbidden"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindRateLimited     Kind = "rate_limited"
	KindUnavailable     Kind = "unavailable"
	KindInternal        Kind = "internal"
)

// Valid reports whether kind is one of the stable application categories.
func (kind Kind) Valid() bool {
	switch kind {
	case KindInvalid,
		KindUnauthenticated,
		KindForbidden,
		KindNotFound,
		KindConflict,
		KindRateLimited,
		KindUnavailable,
		KindInternal:
		return true
	default:
		return false
	}
}

// Error carries a stable public contract while retaining an internal cause.
// Its fields are immutable after construction.
type Error struct {
	kind          Kind
	code          string
	publicMessage string
	cause         error
}

// New validates and constructs a transport-independent failure.
func New(kind Kind, code, publicMessage string, cause error) (*Error, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("create fault: unsupported kind %q", kind)
	}
	if !validCode(code) {
		return nil, fmt.Errorf("create fault: code must match [a-z][a-z0-9_]{0,%d}", maxCodeLength-1)
	}
	if !validPublicMessage(publicMessage) {
		return nil, fmt.Errorf("create fault: public message must be 1-%d printable characters without leading or trailing whitespace", maxPublicMessageLength)
	}

	return &Error{
		kind:          kind,
		code:          code,
		publicMessage: publicMessage,
		cause:         cause,
	}, nil
}

// MustNew is intended for package-level faults whose values are constants.
func MustNew(kind Kind, code, publicMessage string, cause error) *Error {
	fault, err := New(kind, code, publicMessage, cause)
	if err != nil {
		panic(err)
	}
	return fault
}

// Error implements error using only the stable code. Neither the public
// message nor the internal cause is rendered implicitly into logs. The cause
// remains available through errors.Unwrap/errors.Is for explicit handling.
func (fault *Error) Error() string {
	if fault == nil {
		return "<nil>"
	}
	if !fault.contractValid() {
		return "internal_error"
	}
	return fault.code
}

// Unwrap exposes the internal cause to standard error inspection.
func (fault *Error) Unwrap() error {
	if fault == nil {
		return nil
	}
	return fault.cause
}

// Kind returns the stable failure category.
func (fault *Error) Kind() Kind {
	if fault == nil || !fault.contractValid() {
		return KindInternal
	}
	return fault.kind
}

// Code returns the stable machine-readable code.
func (fault *Error) Code() string {
	if fault == nil || !fault.contractValid() {
		return "internal_error"
	}
	return fault.code
}

// PublicMessage returns the client-safe message.
func (fault *Error) PublicMessage() string {
	if fault == nil || !fault.contractValid() {
		return "internal server error"
	}
	return fault.publicMessage
}

// As locates a fault in err's unwrap chain.
func As(err error) (*Error, bool) {
	var target *Error
	if !errors.As(err, &target) || target == nil || !target.contractValid() {
		return nil, false
	}
	return target, true
}

func (fault *Error) contractValid() bool {
	return fault != nil &&
		fault.kind.Valid() &&
		validCode(fault.code) &&
		validPublicMessage(fault.publicMessage)
}

func validCode(code string) bool {
	if len(code) == 0 || len(code) > maxCodeLength || code[0] < 'a' || code[0] > 'z' {
		return false
	}
	for index := 1; index < len(code); index++ {
		character := code[index]
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' {
			return false
		}
	}
	return true
}

func validPublicMessage(message string) bool {
	if message == "" || message != strings.TrimSpace(message) || !utf8.ValidString(message) {
		return false
	}
	if utf8.RuneCountInString(message) > maxPublicMessageLength {
		return false
	}
	for _, character := range message {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
