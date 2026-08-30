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

// Clock supplies one controlled server evaluation instant after a successful
// fact read. Browser and request-supplied timestamps cannot satisfy this port.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function such as time.Now to Clock at a future composition
// boundary without coupling the domain evaluator to the system clock.
type ClockFunc func() time.Time

// Now implements Clock.
func (function ClockFunc) Now() time.Time { return function() }
