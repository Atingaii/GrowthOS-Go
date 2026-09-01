package passwordhash

import (
	"context"
	"log/slog"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
)

// ApplicationVerifier is the narrow dependency-direction bridge from the
// password adapter result into the Identity application port. Keeping this
// mapping here prevents the application layer from importing a concrete
// Argon2 implementation.
type ApplicationVerifier struct {
	hasher *Hasher
}

var _ identityapp.PasswordVerifier = (*ApplicationVerifier)(nil)

// NewApplicationVerifier rejects a nil or partially configured Hasher at
// composition time instead of deferring the failure to a login request.
func NewApplicationVerifier(hasher *Hasher) (*ApplicationVerifier, error) {
	if hasher == nil || hasher.gate == nil {
		return nil, ErrInvalidConfiguration
	}
	return &ApplicationVerifier{hasher: hasher}, nil
}

func (verifier *ApplicationVerifier) String() string {
	return "passwordhash.ApplicationVerifier{[REDACTED]}"
}

func (verifier *ApplicationVerifier) GoString() string { return verifier.String() }

func (verifier *ApplicationVerifier) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

// VerifyLogin performs the concrete bounded hash work and converts only the
// two reviewed booleans into the application-owned result type.
func (verifier *ApplicationVerifier) VerifyLogin(
	ctx context.Context,
	password []byte,
	encodedEnvelope string,
) (identityapp.PasswordVerification, error) {
	if verifier == nil || verifier.hasher == nil || verifier.hasher.gate == nil {
		return identityapp.PasswordVerification{}, ErrInvalidConfiguration
	}
	verification, err := verifier.hasher.VerifyLogin(ctx, password, encodedEnvelope)
	if err != nil {
		return identityapp.PasswordVerification{}, err
	}
	converted, err := identityapp.NewPasswordVerification(
		verification.Matched(),
		verification.NeedsRehash(),
	)
	if err != nil {
		return identityapp.PasswordVerification{}, ErrInvalidConfiguration
	}
	return converted, nil
}

// VerifyUnknownLogin preserves the fixed current-profile dummy path without
// exposing a comparison result to the application layer.
func (verifier *ApplicationVerifier) VerifyUnknownLogin(
	ctx context.Context,
	password []byte,
) error {
	if verifier == nil || verifier.hasher == nil || verifier.hasher.gate == nil {
		return ErrInvalidConfiguration
	}
	return verifier.hasher.VerifyUnknownLogin(ctx, password)
}
