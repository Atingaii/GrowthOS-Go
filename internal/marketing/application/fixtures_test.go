package application

import (
	"context"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

const (
	testActivityID domain.ActivityID = 41
)

var (
	testPublishedAt = time.Date(2026, 8, 30, 12, 0, 0, 123456000, time.UTC)
	testStartsAt    = testPublishedAt.Add(-time.Hour)
	testEndsAt      = testPublishedAt.Add(time.Hour)
)

func testGraphReference(t *testing.T) domain.LotteryGraphReference {
	t.Helper()
	reference, err := domain.NewLotteryGraphReference(7, "graph-r1")
	if err != nil {
		t.Fatalf("new graph reference: %v", err)
	}
	return reference
}

func testStrategyManifest(t *testing.T) []domain.LotteryStrategyRevisionReference {
	t.Helper()
	first, err := domain.NewLotteryStrategyRevisionReference(11, "strategy-r3")
	if err != nil {
		t.Fatalf("new first Strategy reference: %v", err)
	}
	second, err := domain.NewLotteryStrategyRevisionReference(22, "strategy-r8")
	if err != nil {
		t.Fatalf("new second Strategy reference: %v", err)
	}
	return []domain.LotteryStrategyRevisionReference{first, second}
}

func testDraft(t *testing.T) domain.Activity {
	t.Helper()
	activity, err := domain.NewActivity(testActivityID, "Autumn Growth Campaign")
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	return activity
}

func testPublished(
	t *testing.T,
	startsAt time.Time,
	endsAt time.Time,
	publishedAt time.Time,
) (domain.Activity, domain.ActivityPublication) {
	t.Helper()
	transition, err := domain.PlanPublish(
		testDraft(t),
		startsAt,
		endsAt,
		testGraphReference(t),
		testStrategyManifest(t),
		domain.EvidenceReference("approval/release-1"),
		publishedAt,
	)
	if err != nil {
		t.Fatalf("plan published fixture: %v", err)
	}
	publication, ok := transition.Record()
	if !ok {
		t.Fatal("published fixture transition has no record")
	}
	return transition.Next(), publication
}

func testPublishedV2(
	t *testing.T,
) (domain.Activity, domain.ActivityPublication, domain.ActivityPublication) {
	t.Helper()
	currentV1, publicationV1 := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
	transition, err := domain.PlanPublish(
		currentV1,
		testStartsAt.Add(10*time.Minute),
		testEndsAt.Add(10*time.Minute),
		testGraphReference(t),
		testStrategyManifest(t),
		domain.EvidenceReference("approval/release-2"),
		testPublishedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("plan v2 fixture: %v", err)
	}
	publicationV2, ok := transition.Record()
	if !ok {
		t.Fatal("v2 fixture transition has no record")
	}
	return transition.Next(), publicationV1, publicationV2
}

type draftCreatorFunc func(context.Context, domain.Activity) error

func (function draftCreatorFunc) CreateDraft(ctx context.Context, activity domain.Activity) error {
	return function(ctx, activity)
}

type activityReaderFunc func(context.Context, domain.ActivityID) (domain.Activity, error)

func (function activityReaderFunc) FindActivityByID(
	ctx context.Context,
	id domain.ActivityID,
) (domain.Activity, error) {
	return function(ctx, id)
}

type currentReaderFunc func(
	context.Context,
	domain.ActivityID,
) (domain.Activity, domain.ActivityPublication, error)

func (function currentReaderFunc) FindCurrentActivity(
	ctx context.Context,
	id domain.ActivityID,
) (domain.Activity, domain.ActivityPublication, error) {
	return function(ctx, id)
}

type publicationReaderFunc func(
	context.Context,
	domain.ActivityID,
	domain.ActivityPublicationVersion,
) (domain.ActivityPublication, error)

func (function publicationReaderFunc) FindPublicationByIdentity(
	ctx context.Context,
	id domain.ActivityID,
	version domain.ActivityPublicationVersion,
) (domain.ActivityPublication, error) {
	return function(ctx, id, version)
}

type publicationWriterFunc func(context.Context, domain.ActivityTransition) error

func (function publicationWriterFunc) CompareAndSwapPublication(
	ctx context.Context,
	transition domain.ActivityTransition,
) error {
	return function(ctx, transition)
}

type retirerFunc func(context.Context, domain.ActivityTransition) error

func (function retirerFunc) CompareAndSwapRetirement(
	ctx context.Context,
	transition domain.ActivityTransition,
) error {
	return function(ctx, transition)
}

type lotteryVerifierFunc func(context.Context, ActivityPublicationCandidate) error

func (function lotteryVerifierFunc) VerifyPublication(
	ctx context.Context,
	candidate ActivityPublicationCandidate,
) error {
	return function(ctx, candidate)
}

type approvalVerifierStub struct {
	publication func(context.Context, ActivityPublicationCandidate) (domain.EvidenceReference, error)
	retirement  func(context.Context, ActivityRetirementCandidate) (domain.EvidenceReference, error)
}

func (stub *approvalVerifierStub) VerifyPublication(
	ctx context.Context,
	candidate ActivityPublicationCandidate,
) (domain.EvidenceReference, error) {
	return stub.publication(ctx, candidate)
}

func (stub *approvalVerifierStub) VerifyRetirement(
	ctx context.Context,
	candidate ActivityRetirementCandidate,
) (domain.EvidenceReference, error) {
	return stub.retirement(ctx, candidate)
}

type countingClock struct {
	instant time.Time
	calls   int
}

func (clock *countingClock) Now() time.Time {
	clock.calls++
	return clock.instant
}

func acceptingApproval() *approvalVerifierStub {
	return &approvalVerifierStub{
		publication: func(context.Context, ActivityPublicationCandidate) (domain.EvidenceReference, error) {
			return domain.EvidenceReference("governance/publication-accepted"), nil
		},
		retirement: func(context.Context, ActivityRetirementCandidate) (domain.EvidenceReference, error) {
			return domain.EvidenceReference("governance/retirement-accepted"), nil
		},
	}
}

func acceptingLottery() LotteryVerifier {
	return lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error { return nil })
}
