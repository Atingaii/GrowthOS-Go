package application

import "time"

// evaluationInstant is a package-owned token. Keeping the raw time behind this
// type prevents transports and arbitrary callers from selecting an old as-of
// instant to make a stale fact appear fresh.
type evaluationInstant struct {
	value time.Time
}

func captureEvaluationInstant(clock Clock) (evaluationInstant, error) {
	if dependencyIsNil(clock) {
		return evaluationInstant{}, ErrEligibilityNotConfigured
	}
	value := canonicalApplicationInstant(clock.Now())
	if value.IsZero() {
		return evaluationInstant{}, ErrEligibilityClockInvalid
	}
	return evaluationInstant{value: value}, nil
}

func (instant evaluationInstant) validate() error {
	if instant.value.IsZero() || instant.value != canonicalApplicationInstant(instant.value) {
		return ErrEligibilityClockInvalid
	}
	return nil
}

func (instant evaluationInstant) time() time.Time { return instant.value }
