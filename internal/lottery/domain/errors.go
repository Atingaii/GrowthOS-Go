package domain

import "errors"

var (
	// ErrStrategyIDRequired reports that a strategy has no durable identity.
	ErrStrategyIDRequired = errors.New("lottery: strategy id must be positive")
	// ErrStrategyNameRequired reports that a strategy has no meaningful name.
	ErrStrategyNameRequired = errors.New("lottery: strategy name is required")
	// ErrStrategyNameInvalid reports invalid UTF-8 or a control character.
	ErrStrategyNameInvalid = errors.New("lottery: strategy name is invalid")
	// ErrStrategyNameTooLong reports a name beyond MaxStrategyNameRunes.
	ErrStrategyNameTooLong = errors.New("lottery: strategy name is too long")
	// ErrStrategyAwardsRequired reports that a strategy has no selectable outcome.
	ErrStrategyAwardsRequired = errors.New("lottery: strategy requires at least one award")
	// ErrDuplicateAwardID reports that two awards in one strategy share an identity.
	ErrDuplicateAwardID = errors.New("lottery: award id must be unique within a strategy")
	// ErrTotalWeightOverflow reports that award weights cannot be summed safely.
	ErrTotalWeightOverflow = errors.New("lottery: total award weight overflows uint64")

	// ErrAwardIDRequired reports that an award has no durable identity.
	ErrAwardIDRequired = errors.New("lottery: award id must be positive")
	// ErrAwardNameRequired reports that an award has no meaningful display name.
	ErrAwardNameRequired = errors.New("lottery: award name is required")
	// ErrAwardNameInvalid reports invalid UTF-8 or a control character.
	ErrAwardNameInvalid = errors.New("lottery: award name is invalid")
	// ErrAwardNameTooLong reports a name beyond MaxAwardNameRunes.
	ErrAwardNameTooLong = errors.New("lottery: award name is too long")
	// ErrAwardWeightRequired reports that an award can never be selected.
	ErrAwardWeightRequired = errors.New("lottery: award weight must be positive")
	// ErrAwardOutcomeInvalid reports an outcome outside the closed Lottery vocabulary.
	ErrAwardOutcomeInvalid = errors.New("lottery: award outcome is invalid")
)
