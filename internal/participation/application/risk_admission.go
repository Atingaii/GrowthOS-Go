package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/participation/domain"
)

func readRiskScreeningFact(
	ctx context.Context,
	reader RiskScreeningFactReader,
	participantRef domain.ParticipantRef,
) (domain.RiskScreeningFactSnapshot, error) {
	fact, err := reader.FindRiskScreeningFact(ctx, participantRef)
	if contextError := ctx.Err(); contextError != nil {
		return domain.RiskScreeningFactSnapshot{}, contextError
	}
	if err != nil {
		return domain.RiskScreeningFactSnapshot{}, classifyRiskScreeningFactReadError(err)
	}
	return fact, nil
}

func evaluateRiskScreeningFactAt(
	ctx context.Context,
	participantRef domain.ParticipantRef,
	policy domain.RiskAdmissionPolicy,
	fact domain.RiskScreeningFactSnapshot,
	instant evaluationInstant,
	maxFactAge time.Duration,
) (domain.RiskAdmissionDecision, error) {
	if err := instant.validate(); err != nil {
		return domain.RiskAdmissionDecision{}, err
	}
	evaluatedAt := instant.time()
	if err := fact.Validate(); err != nil {
		return domain.RiskAdmissionDecision{}, fmt.Errorf(
			"%w: %w",
			ErrRiskScreeningFactInvalid,
			err,
		)
	}
	if fact.ParticipantRef() != participantRef || fact.AssessedAt().After(evaluatedAt) {
		return domain.RiskAdmissionDecision{}, ErrRiskScreeningFactInvalid
	}
	if maxFactAge <= 0 {
		return domain.RiskAdmissionDecision{}, ErrPrerequisiteChainNotConfigured
	}
	if evaluatedAt.Sub(fact.AssessedAt()) > maxFactAge {
		return domain.RiskAdmissionDecision{}, ErrRiskScreeningFactStale
	}
	if err := ctx.Err(); err != nil {
		return domain.RiskAdmissionDecision{}, err
	}

	decision, err := domain.EvaluateRiskAdmission(policy, fact, evaluatedAt)
	if contextError := ctx.Err(); contextError != nil {
		return domain.RiskAdmissionDecision{}, contextError
	}
	if err != nil {
		return domain.RiskAdmissionDecision{}, fmt.Errorf(
			"%w: %w",
			ErrRiskScreeningFactInvalid,
			err,
		)
	}
	return decision, nil
}

func classifyRiskScreeningFactReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return WrapRiskScreeningFactReadError(ErrRiskScreeningFactUnavailable, err)
	}
	class := ErrRiskScreeningFactReadFailure
	switch {
	case errors.Is(err, ErrRiskScreeningFactNotFound):
		class = ErrRiskScreeningFactNotFound
	case errors.Is(err, ErrRiskScreeningFactUnavailable):
		class = ErrRiskScreeningFactUnavailable
	case errors.Is(err, ErrRiskScreeningFactReadFailure):
		class = ErrRiskScreeningFactReadFailure
	}
	return WrapRiskScreeningFactReadError(class, err)
}
