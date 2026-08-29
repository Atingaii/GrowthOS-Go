package domain

import "errors"

var (
	// ErrSelectorNotConfigured means a WeightedSelector has no bounded random
	// source. It is a composition error and must not be treated as no_reward.
	ErrSelectorNotConfigured = errors.New("lottery selection: selector is not configured")
	// ErrSelectionStrategyInvalid means Select received a zero or otherwise
	// unusable Strategy value instead of an aggregate created by this package.
	ErrSelectionStrategyInvalid = errors.New("lottery selection: strategy is invalid")
	// ErrRandomSourceFailure means the entropy adapter could not produce a
	// bounded value. This selection returns no Award and must fail closed.
	ErrRandomSourceFailure = errors.New("lottery selection: random source failed")
	// ErrRandomSourceContractViolation means an adapter returned a value outside
	// the promised half-open interval [0, upper).
	ErrRandomSourceContractViolation = errors.New("lottery selection: random source violated its contract")
	// ErrSelectionInvariantViolation means a valid ticket could not be mapped to
	// one configured Award. Constructors should make this state unreachable.
	ErrSelectionInvariantViolation = errors.New("lottery selection: strategy mapping invariant was violated")
)

// SelectionError exposes a stable semantic class while retaining a diagnostic
// cause for trusted logs and tests. Its rendered string never includes entropy
// adapter details.
type SelectionError struct {
	class error
	cause error
}

func newSelectionError(class, cause error) *SelectionError {
	if !knownSelectionError(class) {
		class = ErrSelectionInvariantViolation
	}
	return &SelectionError{class: class, cause: cause}
}

func (e *SelectionError) Error() string {
	if e == nil || !knownSelectionError(e.class) {
		return ErrSelectionInvariantViolation.Error()
	}
	return e.class.Error()
}

// Is makes the semantic class usable with errors.Is.
func (e *SelectionError) Is(target error) bool {
	if e == nil || !knownSelectionError(e.class) {
		return target == ErrSelectionInvariantViolation
	}
	return target == e.class
}

// Unwrap retains the source error without exposing it in Error().
func (e *SelectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownSelectionError(class error) bool {
	return class == ErrSelectorNotConfigured ||
		class == ErrSelectionStrategyInvalid ||
		class == ErrRandomSourceFailure ||
		class == ErrRandomSourceContractViolation ||
		class == ErrSelectionInvariantViolation
}
