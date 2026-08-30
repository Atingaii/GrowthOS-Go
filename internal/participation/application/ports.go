package application

import (
	"context"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/participation/domain"
)

// RegistrationFactReader is owned by the eligibility consumer. A future user
// directory adapter must return only the minimum authoritative snapshot and
// classify lookup failures with this package's stable errors.
type RegistrationFactReader interface {
	FindRegistrationFact(
		ctx context.Context,
		participantRef domain.ParticipantRef,
	) (domain.RegistrationFactSnapshot, error)
}

// RiskScreeningFactReader is the Participation-owned view of a controlled risk
// authority. It returns only an immutable screening snapshot; adapters must not
// refresh an old verdict by stamping the local retrieval time onto it.
type RiskScreeningFactReader interface {
	FindRiskScreeningFact(
		ctx context.Context,
		participantRef domain.ParticipantRef,
	) (domain.RiskScreeningFactSnapshot, error)
}

// Clock supplies a controlled server instant. The standalone Lesson 25 service
// captures it after its successful read; the Lesson 26 chain captures it once
// before both lazy reads. Browser and request timestamps cannot satisfy it.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function such as time.Now to Clock at a future composition
// boundary without coupling the domain evaluator to the system clock.
type ClockFunc func() time.Time

// Now implements Clock.
func (function ClockFunc) Now() time.Time { return function() }
