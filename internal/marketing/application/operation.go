package application

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

var errActivityOperationInternalDeadline = errors.New("marketing activity: internal deadline")

func activityOperationContext(
	callerCtx context.Context,
	maxDuration time.Duration,
) (context.Context, context.CancelFunc) {
	internalDeadline := time.Now().Add(maxDuration)
	if callerDeadline, ok := callerCtx.Deadline(); ok && !internalDeadline.Before(callerDeadline) {
		return context.WithCancel(callerCtx)
	}
	return context.WithDeadlineCause(callerCtx, internalDeadline, errActivityOperationInternalDeadline)
}

// activityOperationContextError establishes deterministic precedence after
// every dependency boundary: caller cancellation, private internal timeout,
// then dependency error classification.
func activityOperationContextError(callerCtx, operationCtx context.Context) error {
	if err := callerCtx.Err(); err != nil {
		return err
	}
	if operationCtx.Err() == nil {
		return nil
	}
	cause := context.Cause(operationCtx)
	if cause == errActivityOperationInternalDeadline {
		return wrapActivityOperationError(ErrActivityOperationTimedOut, cause)
	}
	return wrapActivityOperationError(ErrActivityOperationFailure, cause)
}

func canonicalOperationInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0).Truncate(time.Microsecond)
}

func readOperationInstant(
	callerCtx context.Context,
	operationCtx context.Context,
	clock Clock,
) (time.Time, error) {
	instant := canonicalOperationInstant(clock.Now())
	if err := activityOperationContextError(callerCtx, operationCtx); err != nil {
		return time.Time{}, err
	}
	if instant.IsZero() {
		return time.Time{}, ErrActivityClockInvalid
	}
	return instant, nil
}

func validateCurrentActivity(
	activity domain.Activity,
	requestedID domain.ActivityID,
	expectedStateVersion domain.ActivityStateVersion,
) error {
	if err := activity.Validate(); err != nil {
		return wrapActivityOperationError(ErrStoredActivityInvalid, err)
	}
	if activity.ID() != requestedID {
		return wrapActivityOperationError(
			ErrStoredActivityInvalid,
			errors.New("reader returned a different Activity identity"),
		)
	}
	if activity.StateVersion() != expectedStateVersion {
		return wrapActivityOperationError(ErrActivityStateConflict, errors.New("state version differs"))
	}
	return nil
}

func classifyRepositoryOperationError(err error) error {
	class := ErrActivityOperationFailure
	for _, candidate := range []error{
		ErrRepositoryInvalidArgument,
		ErrRepositoryNotConfigured,
		ErrActivityNotFound,
		ErrActivityAlreadyExists,
		ErrActivityPublicationNotFound,
		ErrStoredActivityInvalid,
		ErrStoredActivityPublicationInvalid,
		ErrActivityStateConflict,
		ErrRepositoryRetryable,
		ErrCommitOutcomeUnknown,
		ErrRepositoryFailure,
	} {
		if errors.Is(err, candidate) {
			class = candidate
			break
		}
	}
	return wrapActivityOperationError(class, err)
}

func classifyLotteryVerificationError(err error) error {
	class := ErrLotteryPublicationUnavailable
	if errors.Is(err, ErrLotteryPublicationInvalid) {
		class = ErrLotteryPublicationInvalid
	}
	return wrapActivityOperationError(class, err)
}

func classifyApprovalError(err error) error {
	class := ErrActivityApprovalUnavailable
	if errors.Is(err, ErrActivityApprovalRejected) {
		class = ErrActivityApprovalRejected
	}
	return wrapActivityOperationError(class, err)
}

func validateApprovalEvidence(reference domain.EvidenceReference) error {
	if err := reference.Validate(); err != nil {
		return wrapActivityOperationError(ErrActivityApprovalEvidenceInvalid, err)
	}
	return nil
}

func dependencyIsNil(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice,
		reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}

func publicationIsZero(publication domain.ActivityPublication) bool {
	rollbackOf, rollback := publication.RollbackOf()
	graph := publication.GraphReference()
	return publication.ActivityID() == 0 &&
		publication.Version() == 0 &&
		publication.SchemaVersion() == 0 &&
		publication.Kind() == "" &&
		!rollback && rollbackOf == 0 &&
		publication.StartsAt().IsZero() &&
		publication.EndsAt().IsZero() &&
		publication.PublishedAt().IsZero() &&
		graph.ID() == 0 && graph.Revision() == "" &&
		len(publication.StrategyRevisionManifest()) == 0 &&
		publication.ApprovalEvidenceReference() == ""
}
