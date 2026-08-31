package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

func TestServicesDiscardDependencyValuesReturnedWithErrors(t *testing.T) {
	t.Run("publish discards Activity returned with repository error", func(t *testing.T) {
		downstreamCalls := 0
		service, err := NewPublishActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return testDraft(t), WrapRepositoryError(ErrRepositoryFailure, errors.New("private read failure"))
			}),
			publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
				downstreamCalls++
				return nil
			}),
			lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
				downstreamCalls++
				return nil
			}),
			acceptingApproval(),
			ClockFunc(func() time.Time { downstreamCalls++; return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new publish service: %v", err)
		}
		publication, err := service.Publish(context.Background(), testPublishCommand(t))
		if !errors.Is(err, ErrRepositoryFailure) || !publicationIsZero(publication) {
			t.Fatalf("publication/error = %#v/%v", publication, err)
		}
		if downstreamCalls != 0 {
			t.Fatalf("downstream calls = %d, want zero", downstreamCalls)
		}
	})

	t.Run("rollback discards history returned with repository error", func(t *testing.T) {
		current, target, _ := testPublishedV2(t)
		downstreamCalls := 0
		service, err := NewRollbackActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return current, nil
			}),
			publicationReaderFunc(func(
				context.Context,
				domain.ActivityID,
				domain.ActivityPublicationVersion,
			) (domain.ActivityPublication, error) {
				return target, WrapRepositoryError(ErrRepositoryFailure, errors.New("private history failure"))
			}),
			publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
				downstreamCalls++
				return nil
			}),
			lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
				downstreamCalls++
				return nil
			}),
			acceptingApproval(),
			ClockFunc(func() time.Time { downstreamCalls++; return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new rollback service: %v", err)
		}
		publication, err := service.Rollback(context.Background(), RollbackActivityCommand{
			ActivityID:           testActivityID,
			ExpectedStateVersion: current.StateVersion(),
			TargetVersion:        target.Version(),
		})
		if !errors.Is(err, ErrRepositoryFailure) || !publicationIsZero(publication) {
			t.Fatalf("publication/error = %#v/%v", publication, err)
		}
		if downstreamCalls != 0 {
			t.Fatalf("downstream calls = %d, want zero", downstreamCalls)
		}
	})

	t.Run("resolve discards current snapshot returned with repository error", func(t *testing.T) {
		activity, publication := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
		downstreamCalls := 0
		service, err := NewResolveActivityService(
			currentReaderFunc(func(
				context.Context,
				domain.ActivityID,
			) (domain.Activity, domain.ActivityPublication, error) {
				return activity, publication, WrapRepositoryError(
					ErrRepositoryFailure,
					errors.New("private current failure"),
				)
			}),
			lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
				downstreamCalls++
				return nil
			}),
			ClockFunc(func() time.Time { downstreamCalls++; return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new resolve service: %v", err)
		}
		decision, err := service.Resolve(context.Background(), testActivityID)
		if !errors.Is(err, ErrRepositoryFailure) || decision != (domain.ActivityGateDecision{}) {
			t.Fatalf("decision/error = %#v/%v", decision, err)
		}
		if downstreamCalls != 0 {
			t.Fatalf("downstream calls = %d, want zero", downstreamCalls)
		}
	})
}

func TestServicesFailClosedForPreCancellationAndZeroClock(t *testing.T) {
	t.Run("pre-cancelled caller performs no write", func(t *testing.T) {
		writes := 0
		service, err := NewCreateDraftService(
			draftCreatorFunc(func(context.Context, domain.Activity) error { writes++; return nil }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new create service: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		activity, err := service.Create(ctx, CreateDraftCommand{ActivityID: testActivityID, Name: "Campaign"})
		if !errors.Is(err, context.Canceled) || activity != (domain.Activity{}) || writes != 0 {
			t.Fatalf("activity/error/writes = %#v/%v/%d", activity, err, writes)
		}
	})

	t.Run("publish zero Clock stops before verification and approval", func(t *testing.T) {
		downstreamCalls := 0
		approval := &approvalVerifierStub{
			publication: func(context.Context, ActivityPublicationCandidate) (domain.EvidenceReference, error) {
				downstreamCalls++
				return "governance/unexpected", nil
			},
			retirement: acceptingApproval().retirement,
		}
		service, err := NewPublishActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return testDraft(t), nil
			}),
			publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
				downstreamCalls++
				return nil
			}),
			lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
				downstreamCalls++
				return nil
			}),
			approval,
			ClockFunc(func() time.Time { return time.Time{} }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new publish service: %v", err)
		}
		publication, err := service.Publish(context.Background(), testPublishCommand(t))
		if !errors.Is(err, ErrActivityClockInvalid) || !publicationIsZero(publication) {
			t.Fatalf("publication/error = %#v/%v", publication, err)
		}
		if downstreamCalls != 0 {
			t.Fatalf("downstream calls = %d, want zero", downstreamCalls)
		}
	})

	t.Run("approval error wins over its non-zero evidence", func(t *testing.T) {
		writes := 0
		service, err := NewPublishActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return testDraft(t), nil
			}),
			publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
				writes++
				return nil
			}),
			acceptingLottery(),
			&approvalVerifierStub{
				publication: func(context.Context, ActivityPublicationCandidate) (domain.EvidenceReference, error) {
					return "governance/must-be-discarded", WrapApprovalError(
						ErrActivityApprovalRejected,
						errors.New("private rejection"),
					)
				},
				retirement: acceptingApproval().retirement,
			},
			ClockFunc(func() time.Time { return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new publish service: %v", err)
		}
		publication, err := service.Publish(context.Background(), testPublishCommand(t))
		if !errors.Is(err, ErrActivityApprovalRejected) || !publicationIsZero(publication) || writes != 0 {
			t.Fatalf("publication/error/writes = %#v/%v/%d", publication, err, writes)
		}
	})
}

func testPublishCommand(t *testing.T) PublishActivityCommand {
	t.Helper()
	return PublishActivityCommand{
		ActivityID:               testActivityID,
		ExpectedStateVersion:     0,
		StartsAt:                 testStartsAt,
		EndsAt:                   testEndsAt,
		GraphReference:           testGraphReference(t),
		StrategyRevisionManifest: testStrategyManifest(t),
	}
}
