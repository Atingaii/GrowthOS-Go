package application

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

func TestCreateDraftServicePersistsOnlyValidDraft(t *testing.T) {
	var stored domain.Activity
	service, err := NewCreateDraftService(
		draftCreatorFunc(func(_ context.Context, activity domain.Activity) error {
			stored = activity
			return nil
		}),
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := service.Create(context.Background(), CreateDraftCommand{
		ActivityID: testActivityID,
		Name:       "  Autumn Growth Campaign  ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID() != testActivityID || created.Name().String() != "Autumn Growth Campaign" {
		t.Fatalf("created = %#v, want canonical Activity", created)
	}
	if created.Lifecycle() != domain.ActivityLifecycleDraft || created.StateVersion() != 0 {
		t.Fatalf("created lifecycle/state = %q/%d", created.Lifecycle(), created.StateVersion())
	}
	if stored.ID() != created.ID() || stored.Name() != created.Name() {
		t.Fatalf("stored = %#v, created = %#v", stored, created)
	}
}

func TestCreateDraftServiceFailureReturnsZeroAndPreservesClass(t *testing.T) {
	secret := errors.New("duplicate SQL INSERT including secret-dsn")
	service, err := NewCreateDraftService(
		draftCreatorFunc(func(context.Context, domain.Activity) error {
			return WrapRepositoryError(ErrActivityAlreadyExists, secret)
		}),
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := service.Create(context.Background(), CreateDraftCommand{
		ActivityID: testActivityID,
		Name:       "Campaign",
	})
	if !errors.Is(err, ErrActivityAlreadyExists) {
		t.Fatalf("error = %v, want already exists", err)
	}
	if created != (domain.Activity{}) {
		t.Fatalf("created = %#v, want zero", created)
	}
	if err.Error() != ErrActivityAlreadyExists.Error() || errors.Is(err, secret) {
		t.Fatalf("error leaked cause: %q", err)
	}
}

func TestPublishActivityServiceOrdersExactVerificationApprovalAndCAS(t *testing.T) {
	draft := testDraft(t)
	clock := &countingClock{instant: testPublishedAt.Add(789 * time.Nanosecond)}
	var trace []string
	var verified ActivityPublicationCandidate
	var persisted domain.ActivityTransition
	service, err := NewPublishActivityService(
		activityReaderFunc(func(_ context.Context, id domain.ActivityID) (domain.Activity, error) {
			trace = append(trace, "read")
			if id != testActivityID {
				t.Fatalf("read id = %d", id)
			}
			return draft, nil
		}),
		publicationWriterFunc(func(_ context.Context, transition domain.ActivityTransition) error {
			trace = append(trace, "cas")
			persisted = transition
			return nil
		}),
		lotteryVerifierFunc(func(_ context.Context, candidate ActivityPublicationCandidate) error {
			trace = append(trace, "lottery")
			verified = candidate
			if candidate.ActivityID() != testActivityID ||
				candidate.SchemaVersion() != domain.ActivityPublicationSchemaVersionV1 ||
				candidate.Kind() != domain.ActivityPublicationKindRelease {
				t.Fatalf("candidate identity/schema/kind = %d/%d/%q", candidate.ActivityID(), candidate.SchemaVersion(), candidate.Kind())
			}
			if rollbackOf, rollback := candidate.RollbackOf(); rollback || rollbackOf != 0 {
				t.Fatalf("release rollback shape = %d/%t", rollbackOf, rollback)
			}
			return nil
		}),
		&approvalVerifierStub{
			publication: func(_ context.Context, candidate ActivityPublicationCandidate) (domain.EvidenceReference, error) {
				trace = append(trace, "approval")
				if !sameActivityPublicationCandidate(verified, candidate) {
					t.Fatal("approval candidate differs from Lottery candidate")
				}
				return domain.EvidenceReference("governance/release-41-1"), nil
			},
			retirement: acceptingApproval().retirement,
		},
		clock,
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	manifest := testStrategyManifest(t)
	manifest[0], manifest[1] = manifest[1], manifest[0]

	publication, err := service.Publish(context.Background(), PublishActivityCommand{
		ActivityID:               testActivityID,
		ExpectedStateVersion:     0,
		StartsAt:                 testStartsAt,
		EndsAt:                   testEndsAt,
		GraphReference:           testGraphReference(t),
		StrategyRevisionManifest: manifest,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !slices.Equal(trace, []string{"read", "lottery", "approval", "cas"}) {
		t.Fatalf("trace = %v", trace)
	}
	if clock.calls != 1 {
		t.Fatalf("Clock calls = %d, want 1", clock.calls)
	}
	if publication.Version() != 1 || publication.Kind() != domain.ActivityPublicationKindRelease {
		t.Fatalf("publication version/kind = %d/%q", publication.Version(), publication.Kind())
	}
	if publication.PublishedAt() != canonicalOperationInstant(clock.instant) {
		t.Fatalf("published-at = %v", publication.PublishedAt())
	}
	if publication.ApprovalEvidenceReference() != "governance/release-41-1" {
		t.Fatalf("evidence = %q", publication.ApprovalEvidenceReference())
	}
	if err := persisted.Validate(); err != nil {
		t.Fatalf("persisted transition: %v", err)
	}
	if persisted.Next().StateVersion() != 1 || persisted.Next().ActivePublicationVersion() != 1 {
		t.Fatalf("next state/active = %d/%d", persisted.Next().StateVersion(), persisted.Next().ActivePublicationVersion())
	}
	returnedManifest := verified.StrategyRevisionManifest()
	returnedManifest[0] = domain.LotteryStrategyRevisionReference{}
	if verified.StrategyRevisionManifest()[0].StrategyID() == 0 {
		t.Fatal("candidate manifest was mutated through accessor")
	}
}

func TestPublishActivityServiceFailsClosedBeforeApprovalOrCAS(t *testing.T) {
	draft := testDraft(t)
	clock := &countingClock{instant: testPublishedAt}
	approvalCalls := 0
	writeCalls := 0
	service, err := NewPublishActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
			return draft, nil
		}),
		publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
			writeCalls++
			return nil
		}),
		lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
			return WrapLotteryVerificationError(ErrLotteryPublicationInvalid, errors.New("missing graph row"))
		}),
		&approvalVerifierStub{
			publication: func(context.Context, ActivityPublicationCandidate) (domain.EvidenceReference, error) {
				approvalCalls++
				return "governance/unexpected", nil
			},
			retirement: acceptingApproval().retirement,
		},
		clock,
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	publication, err := service.Publish(context.Background(), PublishActivityCommand{
		ActivityID:               testActivityID,
		ExpectedStateVersion:     0,
		StartsAt:                 testStartsAt,
		EndsAt:                   testEndsAt,
		GraphReference:           testGraphReference(t),
		StrategyRevisionManifest: testStrategyManifest(t),
	})
	if !errors.Is(err, ErrLotteryPublicationInvalid) {
		t.Fatalf("error = %v, want Lottery invalid", err)
	}
	if publication.ActivityID() != 0 || approvalCalls != 0 || writeCalls != 0 {
		t.Fatalf("partial result/calls = %#v/%d/%d", publication, approvalCalls, writeCalls)
	}
}

func TestPublishActivityServiceExpectedStateConflictStopsBeforeClock(t *testing.T) {
	draft := testDraft(t)
	clock := &countingClock{instant: testPublishedAt}
	service, err := NewPublishActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) { return draft, nil }),
		publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
			t.Fatal("writer must not run")
			return nil
		}),
		lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
			t.Fatal("Lottery verifier must not run")
			return nil
		}),
		acceptingApproval(),
		clock,
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	publication, err := service.Publish(context.Background(), PublishActivityCommand{
		ActivityID:               testActivityID,
		ExpectedStateVersion:     9,
		StartsAt:                 testStartsAt,
		EndsAt:                   testEndsAt,
		GraphReference:           testGraphReference(t),
		StrategyRevisionManifest: testStrategyManifest(t),
	})
	if !errors.Is(err, ErrActivityStateConflict) || publication.ActivityID() != 0 {
		t.Fatalf("publication/error = %#v/%v", publication, err)
	}
	if clock.calls != 0 {
		t.Fatalf("Clock calls = %d, want 0", clock.calls)
	}
}

func TestRollbackActivityServiceCopiesScheduledSourceAndAppendsVersion(t *testing.T) {
	current, target, _ := testPublishedV2(t)
	rollbackAt := target.StartsAt().Add(-time.Minute)
	clock := &countingClock{instant: rollbackAt}
	var persisted domain.ActivityTransition
	service, err := NewRollbackActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) { return current, nil }),
		publicationReaderFunc(func(_ context.Context, id domain.ActivityID, version domain.ActivityPublicationVersion) (domain.ActivityPublication, error) {
			if id != testActivityID || version != target.Version() {
				t.Fatalf("history identity = %d/%d", id, version)
			}
			return target, nil
		}),
		publicationWriterFunc(func(_ context.Context, transition domain.ActivityTransition) error {
			persisted = transition
			return nil
		}),
		acceptingLottery(),
		acceptingApproval(),
		clock,
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	publication, err := service.Rollback(context.Background(), RollbackActivityCommand{
		ActivityID:           testActivityID,
		ExpectedStateVersion: current.StateVersion(),
		TargetVersion:        target.Version(),
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if clock.calls != 1 || publication.Version() != current.ActivePublicationVersion()+1 {
		t.Fatalf("Clock/version = %d/%d", clock.calls, publication.Version())
	}
	rollbackOf, ok := publication.RollbackOf()
	if !ok || rollbackOf != target.Version() {
		t.Fatalf("rollback-of = %d/%t", rollbackOf, ok)
	}
	if publication.StartsAt() != target.StartsAt() ||
		publication.EndsAt() != target.EndsAt() ||
		publication.GraphReference() != target.GraphReference() ||
		!slices.Equal(publication.StrategyRevisionManifest(), target.StrategyRevisionManifest()) {
		t.Fatal("rollback did not copy exact source content")
	}
	if err := persisted.Validate(); err != nil {
		t.Fatalf("persisted transition: %v", err)
	}
}

func TestRollbackActivityServiceRejectsEndedSourceBeforeVerifiers(t *testing.T) {
	current, target, _ := testPublishedV2(t)
	clock := &countingClock{instant: target.EndsAt()}
	verificationCalls := 0
	service, err := NewRollbackActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) { return current, nil }),
		publicationReaderFunc(func(context.Context, domain.ActivityID, domain.ActivityPublicationVersion) (domain.ActivityPublication, error) {
			return target, nil
		}),
		publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
			t.Fatal("writer must not run")
			return nil
		}),
		lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
			verificationCalls++
			return nil
		}),
		acceptingApproval(),
		clock,
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	publication, err := service.Rollback(context.Background(), RollbackActivityCommand{
		ActivityID:           testActivityID,
		ExpectedStateVersion: current.StateVersion(),
		TargetVersion:        target.Version(),
	})
	if !errors.Is(err, ErrActivityRollbackTargetInvalid) || publication.ActivityID() != 0 {
		t.Fatalf("publication/error = %#v/%v", publication, err)
	}
	if verificationCalls != 0 {
		t.Fatalf("verification calls = %d", verificationCalls)
	}
}

func TestRetireActivityServiceApprovesThenCASAndRetainsActive(t *testing.T) {
	current, _ := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
	clock := &countingClock{instant: testPublishedAt.Add(15 * time.Minute)}
	var trace []string
	service, err := NewRetireActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
			trace = append(trace, "read")
			return current, nil
		}),
		retirerFunc(func(_ context.Context, transition domain.ActivityTransition) error {
			trace = append(trace, "cas")
			if transition.AppendsPublication() {
				t.Fatal("retirement appended publication")
			}
			return nil
		}),
		&approvalVerifierStub{
			publication: acceptingApproval().publication,
			retirement: func(_ context.Context, candidate ActivityRetirementCandidate) (domain.EvidenceReference, error) {
				trace = append(trace, "approval")
				if candidate.ActivityID() != testActivityID ||
					candidate.ExpectedStateVersion() != current.StateVersion() ||
					candidate.ActivePublicationVersion() != current.ActivePublicationVersion() {
					t.Fatal("retirement candidate changed active version")
				}
				return "governance/retire-41", nil
			},
		},
		clock,
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	retired, err := service.Retire(context.Background(), RetireActivityCommand{
		ActivityID:           testActivityID,
		ExpectedStateVersion: current.StateVersion(),
	})
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if !slices.Equal(trace, []string{"read", "approval", "cas"}) || clock.calls != 1 {
		t.Fatalf("trace/Clock = %v/%d", trace, clock.calls)
	}
	if retired.Lifecycle() != domain.ActivityLifecycleRetired ||
		retired.ActivePublicationVersion() != current.ActivePublicationVersion() ||
		retired.StateVersion() != current.StateVersion()+1 {
		t.Fatalf("retired = %#v", retired)
	}
}

func TestConcurrentPublishersExposeOneWinnerAndDoNotReplay(t *testing.T) {
	draft := testDraft(t)
	var lock sync.Mutex
	committed := false
	writes := 0
	service, err := NewPublishActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) { return draft, nil }),
		publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
			lock.Lock()
			defer lock.Unlock()
			writes++
			if committed {
				return WrapRepositoryError(ErrActivityStateConflict, errors.New("CAS lost"))
			}
			committed = true
			return nil
		}),
		acceptingLottery(),
		acceptingApproval(),
		ClockFunc(func() time.Time { return testPublishedAt }),
		time.Second,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	command := PublishActivityCommand{
		ActivityID:               testActivityID,
		ExpectedStateVersion:     0,
		StartsAt:                 testStartsAt,
		EndsAt:                   testEndsAt,
		GraphReference:           testGraphReference(t),
		StrategyRevisionManifest: testStrategyManifest(t),
	}
	type result struct {
		publication domain.ActivityPublication
		err         error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			publication, err := service.Publish(context.Background(), command)
			results <- result{publication: publication, err: err}
		}()
	}
	winners := 0
	conflicts := 0
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			winners++
			if result.publication.Version() != 1 {
				t.Fatalf("winner version = %d", result.publication.Version())
			}
		case errors.Is(result.err, ErrActivityStateConflict):
			conflicts++
			if result.publication.ActivityID() != 0 {
				t.Fatalf("conflict exposed publication %#v", result.publication)
			}
		default:
			t.Fatalf("unexpected publish error: %v", result.err)
		}
	}
	if winners != 1 || conflicts != 1 || writes != 2 {
		t.Fatalf("winners/conflicts/writes = %d/%d/%d", winners, conflicts, writes)
	}
}

func TestResolveActivityServiceFormsDraftAndActiveDecisions(t *testing.T) {
	t.Run("draft does not call Lottery", func(t *testing.T) {
		clock := &countingClock{instant: testPublishedAt}
		service, err := NewResolveActivityService(
			currentReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, domain.ActivityPublication, error) {
				return testDraft(t), domain.ActivityPublication{}, nil
			}),
			lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
				t.Fatal("draft must not call Lottery verifier")
				return nil
			}),
			clock,
			time.Second,
		)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		decision, err := service.Resolve(context.Background(), testActivityID)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if decision.Status() != domain.ActivityGateStatusNotPublished || decision.AllowsParticipation() {
			t.Fatalf("decision = %#v", decision)
		}
		if clock.calls != 1 {
			t.Fatalf("Clock calls = %d", clock.calls)
		}
	})

	t.Run("published verifies exact content before one Clock", func(t *testing.T) {
		activity, publication := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
		var trace []string
		clock := ClockFunc(func() time.Time {
			trace = append(trace, "clock")
			return testPublishedAt
		})
		service, err := NewResolveActivityService(
			currentReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, domain.ActivityPublication, error) {
				trace = append(trace, "current")
				return activity, publication, nil
			}),
			lotteryVerifierFunc(func(_ context.Context, candidate ActivityPublicationCandidate) error {
				trace = append(trace, "lottery")
				if candidate.Version() != publication.Version() {
					t.Fatal("resolver verified another version")
				}
				return nil
			}),
			clock,
			time.Second,
		)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		decision, err := service.Resolve(context.Background(), testActivityID)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if decision.Status() != domain.ActivityGateStatusActive || !decision.AllowsParticipation() {
			t.Fatalf("decision = %#v", decision)
		}
		if !slices.Equal(trace, []string{"current", "lottery", "clock"}) {
			t.Fatalf("trace = %v", trace)
		}
	})

	t.Run("retired still verifies retained exact publication", func(t *testing.T) {
		published, publication := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
		transition, err := domain.PlanRetire(
			published,
			domain.EvidenceReference("governance/retire-fixture"),
			testPublishedAt.Add(time.Minute),
		)
		if err != nil {
			t.Fatalf("plan retired fixture: %v", err)
		}
		lotteryCalls := 0
		service, err := NewResolveActivityService(
			currentReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, domain.ActivityPublication, error) {
				return transition.Next(), publication, nil
			}),
			lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
				lotteryCalls++
				return nil
			}),
			ClockFunc(func() time.Time { return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		decision, err := service.Resolve(context.Background(), testActivityID)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if decision.Status() != domain.ActivityGateStatusRetired || lotteryCalls != 1 {
			t.Fatalf("status/calls = %q/%d", decision.Status(), lotteryCalls)
		}
	})

	t.Run("invalid Lottery ref returns zero before Clock", func(t *testing.T) {
		activity, publication := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
		clockCalls := 0
		service, err := NewResolveActivityService(
			currentReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, domain.ActivityPublication, error) {
				return activity, publication, nil
			}),
			lotteryVerifierFunc(func(context.Context, ActivityPublicationCandidate) error {
				return WrapLotteryVerificationError(ErrLotteryPublicationInvalid, errors.New("dangling cross-context ref"))
			}),
			ClockFunc(func() time.Time { clockCalls++; return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		decision, err := service.Resolve(context.Background(), testActivityID)
		if !errors.Is(err, ErrLotteryPublicationInvalid) || decision.ActivityID() != 0 || clockCalls != 0 {
			t.Fatalf("decision/error/Clock = %#v/%v/%d", decision, err, clockCalls)
		}
	})
}

func TestPublicationWriterFailuresReturnZeroWithoutAutomaticRetry(t *testing.T) {
	for _, test := range []struct {
		name  string
		class error
	}{
		{name: "CAS conflict", class: ErrActivityStateConflict},
		{name: "retryable", class: ErrRepositoryRetryable},
		{name: "commit outcome unknown", class: ErrCommitOutcomeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			writes := 0
			service, err := NewPublishActivityService(
				activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) { return testDraft(t), nil }),
				publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
					writes++
					return WrapRepositoryError(test.class, errors.New("private transaction cause"))
				}),
				acceptingLottery(),
				acceptingApproval(),
				ClockFunc(func() time.Time { return testPublishedAt }),
				time.Second,
			)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			publication, err := service.Publish(context.Background(), PublishActivityCommand{
				ActivityID:               testActivityID,
				ExpectedStateVersion:     0,
				StartsAt:                 testStartsAt,
				EndsAt:                   testEndsAt,
				GraphReference:           testGraphReference(t),
				StrategyRevisionManifest: testStrategyManifest(t),
			})
			if !errors.Is(err, test.class) || publication.ActivityID() != 0 || writes != 1 {
				t.Fatalf("publication/error/writes = %#v/%v/%d", publication, err, writes)
			}
		})
	}
}

func TestServicesRejectTypedNilAndNonPositiveDuration(t *testing.T) {
	var typedNilDraftCreator draftCreatorFunc
	if _, err := NewCreateDraftService(typedNilDraftCreator, time.Second); !errors.Is(err, ErrActivityNotConfigured) {
		t.Fatalf("typed-nil create error = %v", err)
	}
	var typedNilClock ClockFunc
	if _, err := NewPublishActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) { return testDraft(t), nil }),
		publicationWriterFunc(func(context.Context, domain.ActivityTransition) error { return nil }),
		acceptingLottery(),
		acceptingApproval(),
		typedNilClock,
		time.Second,
	); !errors.Is(err, ErrActivityNotConfigured) {
		t.Fatalf("typed-nil Clock error = %v", err)
	}
	if _, err := NewResolveActivityService(
		currentReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, domain.ActivityPublication, error) {
			return domain.Activity{}, domain.ActivityPublication{}, nil
		}),
		acceptingLottery(),
		ClockFunc(func() time.Time { return testPublishedAt }),
		0,
	); !errors.Is(err, ErrActivityNotConfigured) {
		t.Fatalf("zero duration error = %v", err)
	}
}

func TestActivityOperationContextPrioritiesReturnZero(t *testing.T) {
	t.Run("caller cancellation beats dependency failure", func(t *testing.T) {
		callerCtx, cancel := context.WithCancel(context.Background())
		service, err := NewCreateDraftService(
			draftCreatorFunc(func(context.Context, domain.Activity) error {
				cancel()
				return WrapRepositoryError(ErrRepositoryFailure, errors.New("driver secret"))
			}),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		activity, err := service.Create(callerCtx, CreateDraftCommand{ActivityID: testActivityID, Name: "Campaign"})
		if !errors.Is(err, context.Canceled) || activity != (domain.Activity{}) {
			t.Fatalf("activity/error = %#v/%v", activity, err)
		}
	})

	t.Run("internal deadline beats dependency result", func(t *testing.T) {
		service, err := NewCreateDraftService(
			draftCreatorFunc(func(ctx context.Context, _ domain.Activity) error {
				<-ctx.Done()
				return WrapRepositoryError(ErrRepositoryFailure, errors.New("late driver error"))
			}),
			5*time.Millisecond,
		)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		activity, err := service.Create(context.Background(), CreateDraftCommand{ActivityID: testActivityID, Name: "Campaign"})
		if !errors.Is(err, ErrActivityOperationTimedOut) ||
			errors.Is(err, context.DeadlineExceeded) ||
			activity != (domain.Activity{}) {
			t.Fatalf("activity/error = %#v/%v", activity, err)
		}
	})
}

func TestApprovalErrorAndEvidenceAlwaysFailClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		evidence domain.EvidenceReference
		err      error
		want     error
	}{
		{name: "rejected", err: WrapApprovalError(ErrActivityApprovalRejected, errors.New("private review")), want: ErrActivityApprovalRejected},
		{name: "unavailable", err: errors.New("provider transport"), want: ErrActivityApprovalUnavailable},
		{name: "invalid evidence", evidence: "bad evidence with spaces", want: ErrActivityApprovalEvidenceInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeCalls := 0
			service, err := NewPublishActivityService(
				activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) { return testDraft(t), nil }),
				publicationWriterFunc(func(context.Context, domain.ActivityTransition) error { writeCalls++; return nil }),
				acceptingLottery(),
				&approvalVerifierStub{
					publication: func(context.Context, ActivityPublicationCandidate) (domain.EvidenceReference, error) {
						return test.evidence, test.err
					},
					retirement: acceptingApproval().retirement,
				},
				ClockFunc(func() time.Time { return testPublishedAt }),
				time.Second,
			)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			publication, err := service.Publish(context.Background(), PublishActivityCommand{
				ActivityID:               testActivityID,
				ExpectedStateVersion:     0,
				StartsAt:                 testStartsAt,
				EndsAt:                   testEndsAt,
				GraphReference:           testGraphReference(t),
				StrategyRevisionManifest: testStrategyManifest(t),
			})
			if !errors.Is(err, test.want) || publication.ActivityID() != 0 || writeCalls != 0 {
				t.Fatalf("publication/error/writes = %#v/%v/%d", publication, err, writeCalls)
			}
		})
	}
}
