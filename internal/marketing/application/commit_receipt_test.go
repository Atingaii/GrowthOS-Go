package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

func TestCommitOutcomeUnknownReturnsZeroWithExactTrustedReceipts(t *testing.T) {
	secret := errors.New("private mysql COMMIT socket failure with credential")

	t.Run("publish", func(t *testing.T) {
		before := testDraft(t)
		service, err := NewPublishActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return before, nil
			}),
			publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
				return WrapRepositoryError(ErrCommitOutcomeUnknown, secret)
			}),
			acceptingLottery(),
			acceptingApproval(),
			ClockFunc(func() time.Time { return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new publish service: %v", err)
		}

		result, operationErr := service.Publish(context.Background(), testPublishCommand(t))
		if !publicationIsZero(result) || !errors.Is(operationErr, ErrCommitOutcomeUnknown) {
			t.Fatalf("result/error = %#v/%v", result, operationErr)
		}
		receipt := requireCommitReceipt(t, operationErr, ActivityCommitOperationPublish)
		if !sameActivity(receipt.Before(), before) ||
			receipt.After().Lifecycle() != domain.ActivityLifecyclePublished {
			t.Fatalf("publish receipt roots = %#v -> %#v", receipt.Before(), receipt.After())
		}
		publication, ok := receipt.Publication()
		if !ok || publication.PublishedAt() != testPublishedAt ||
			publication.ApprovalEvidenceReference() != "governance/publication-accepted" {
			t.Fatalf("publish receipt publication = %#v/%t", publication, ok)
		}
		assertLowDisclosureCommitError(t, operationErr, secret)
	})

	t.Run("rollback", func(t *testing.T) {
		before, target, _ := testPublishedV2(t)
		rollbackAt := target.StartsAt().Add(-time.Minute)
		service, err := NewRollbackActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return before, nil
			}),
			publicationReaderFunc(func(
				context.Context,
				domain.ActivityID,
				domain.ActivityPublicationVersion,
			) (domain.ActivityPublication, error) {
				return target, nil
			}),
			publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
				return WrapRepositoryError(ErrCommitOutcomeUnknown, secret)
			}),
			acceptingLottery(),
			acceptingApproval(),
			ClockFunc(func() time.Time { return rollbackAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new rollback service: %v", err)
		}

		result, operationErr := service.Rollback(context.Background(), RollbackActivityCommand{
			ActivityID:           testActivityID,
			ExpectedStateVersion: before.StateVersion(),
			TargetVersion:        target.Version(),
		})
		if !publicationIsZero(result) || !errors.Is(operationErr, ErrCommitOutcomeUnknown) {
			t.Fatalf("result/error = %#v/%v", result, operationErr)
		}
		receipt := requireCommitReceipt(t, operationErr, ActivityCommitOperationRollback)
		publication, ok := receipt.Publication()
		rollbackOf, rollback := publication.RollbackOf()
		if !ok || !rollback || rollbackOf != target.Version() ||
			publication.PublishedAt() != rollbackAt ||
			publication.ApprovalEvidenceReference() != "governance/publication-accepted" {
			t.Fatalf("rollback receipt publication = %#v/%t", publication, ok)
		}
		if !sameActivity(receipt.Before(), before) ||
			receipt.After().StateVersion() != before.StateVersion()+1 {
			t.Fatal("rollback receipt did not preserve exact roots")
		}
		assertLowDisclosureCommitError(t, operationErr, secret)
	})

	t.Run("retire", func(t *testing.T) {
		before, _ := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
		retiredAt := testPublishedAt.Add(20 * time.Minute)
		service, err := NewRetireActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return before, nil
			}),
			retirerFunc(func(context.Context, domain.ActivityTransition) error {
				return WrapRepositoryError(ErrCommitOutcomeUnknown, secret)
			}),
			acceptingApproval(),
			ClockFunc(func() time.Time { return retiredAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new retire service: %v", err)
		}

		result, operationErr := service.Retire(context.Background(), RetireActivityCommand{
			ActivityID:           testActivityID,
			ExpectedStateVersion: before.StateVersion(),
		})
		if result != (domain.Activity{}) || !errors.Is(operationErr, ErrCommitOutcomeUnknown) {
			t.Fatalf("result/error = %#v/%v", result, operationErr)
		}
		receipt := requireCommitReceipt(t, operationErr, ActivityCommitOperationRetire)
		if _, ok := receipt.Publication(); ok {
			t.Fatal("retirement receipt exposed a publication")
		}
		actualRetiredAt, hasRetiredAt := receipt.After().RetiredAt()
		retirementEvidence, hasEvidence := receipt.After().RetirementReference()
		if !sameActivity(receipt.Before(), before) || !hasRetiredAt || actualRetiredAt != retiredAt ||
			!hasEvidence || retirementEvidence != "governance/retirement-accepted" {
			t.Fatalf("retirement receipt after = %#v", receipt.After())
		}
		assertLowDisclosureCommitError(t, operationErr, secret)
	})
}

func TestCommitReceiptAccessorIsExplicitVerifiedAndDefensive(t *testing.T) {
	receipt, operationErr := testUnknownPublishReceipt(t)
	publication, ok := receipt.Publication()
	if !ok {
		t.Fatal("publish receipt has no publication")
	}
	originalManifest := publication.StrategyRevisionManifest()
	mutated := publication.StrategyRevisionManifest()
	mutated[0] = domain.LotteryStrategyRevisionReference{}

	// Mutating the returned receipt value and accessor slice cannot alter the
	// private copy retained by the operation error.
	receipt.publication = domain.ActivityPublication{}
	again, ok := ActivityCommitReceiptFromError(operationErr)
	if !ok || again.Validate() != nil {
		t.Fatalf("second receipt = %#v/%t", again, ok)
	}
	againPublication, ok := again.Publication()
	if !ok || !slices.Equal(againPublication.StrategyRevisionManifest(), originalManifest) {
		t.Fatal("receipt publication aliased an earlier accessor")
	}

	outer := fmt.Errorf("trusted recovery boundary: %w", operationErr)
	if _, ok := ActivityCommitReceiptFromError(outer); !ok {
		t.Fatal("explicit accessor did not inspect an ordinary outer wrapper")
	}
	if _, ok := ActivityCommitReceiptFromError(errors.New("unrelated")); ok {
		t.Fatal("unrelated error exposed a receipt")
	}

	operation := operationErr.(*ActivityOperationError)
	operation.receipt.operation = "future"
	if _, ok := operation.CommitReceipt(); ok {
		t.Fatal("corrupt retained receipt escaped validation")
	}
}

func TestCommitReceiptOnlyAccompaniesLiveCommitOutcomeUnknown(t *testing.T) {
	validReceipt, _ := testUnknownPublishReceipt(t)
	for _, test := range []struct {
		name  string
		class error
	}{
		{name: "conflict", class: ErrActivityStateConflict},
		{name: "retryable", class: ErrRepositoryRetryable},
		{name: "repository failure", class: ErrRepositoryFailure},
		{name: "unknown unclassified failure", class: errors.New("transport")},
	} {
		t.Run(test.name, func(t *testing.T) {
			classified := classifyRepositoryOperationErrorWithCommitReceipt(
				WrapRepositoryError(test.class, errors.New("private cause")),
				validReceipt,
			)
			if _, ok := ActivityCommitReceiptFromError(classified); ok {
				t.Fatalf("%v unexpectedly carried a receipt", classified)
			}
		})
	}

	t.Run("caller cancellation has priority", func(t *testing.T) {
		callerCtx, cancel := context.WithCancel(context.Background())
		service, err := NewPublishActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return testDraft(t), nil
			}),
			publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
				cancel()
				return WrapRepositoryError(ErrCommitOutcomeUnknown, errors.New("late unknown"))
			}),
			acceptingLottery(),
			acceptingApproval(),
			ClockFunc(func() time.Time { return testPublishedAt }),
			time.Second,
		)
		if err != nil {
			t.Fatalf("new publish service: %v", err)
		}
		result, operationErr := service.Publish(callerCtx, testPublishCommand(t))
		if !publicationIsZero(result) || !errors.Is(operationErr, context.Canceled) {
			t.Fatalf("result/error = %#v/%v", result, operationErr)
		}
		if _, ok := ActivityCommitReceiptFromError(operationErr); ok {
			t.Fatal("cancelled caller received a receipt")
		}
	})

	t.Run("private deadline has priority", func(t *testing.T) {
		service, err := NewPublishActivityService(
			activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
				return testDraft(t), nil
			}),
			publicationWriterFunc(func(ctx context.Context, _ domain.ActivityTransition) error {
				<-ctx.Done()
				return WrapRepositoryError(ErrCommitOutcomeUnknown, errors.New("late unknown"))
			}),
			acceptingLottery(),
			acceptingApproval(),
			ClockFunc(func() time.Time { return testPublishedAt }),
			5*time.Millisecond,
		)
		if err != nil {
			t.Fatalf("new publish service: %v", err)
		}
		result, operationErr := service.Publish(context.Background(), testPublishCommand(t))
		if !publicationIsZero(result) || !errors.Is(operationErr, ErrActivityOperationTimedOut) {
			t.Fatalf("result/error = %#v/%v", result, operationErr)
		}
		if _, ok := ActivityCommitReceiptFromError(operationErr); ok {
			t.Fatal("timed-out operation received a receipt")
		}
	})
}

func TestEveryHighRiskServiceReturnsZeroWithoutReceiptForOrdinaryFailureOrCancellation(t *testing.T) {
	type invoke func(dependencyErr error, cancelAtWrite bool) (bool, error)
	operations := []struct {
		name   string
		invoke invoke
	}{
		{
			name: "publish",
			invoke: func(dependencyErr error, cancelAtWrite bool) (bool, error) {
				callerCtx := context.Background()
				cancel := context.CancelFunc(func() {})
				if cancelAtWrite {
					callerCtx, cancel = context.WithCancel(callerCtx)
				}
				defer cancel()
				service, err := NewPublishActivityService(
					activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
						return testDraft(t), nil
					}),
					publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
						if cancelAtWrite {
							cancel()
						}
						return dependencyErr
					}),
					acceptingLottery(),
					acceptingApproval(),
					ClockFunc(func() time.Time { return testPublishedAt }),
					time.Second,
				)
				if err != nil {
					t.Fatalf("new publish service: %v", err)
				}
				result, operationErr := service.Publish(callerCtx, testPublishCommand(t))
				return publicationIsZero(result), operationErr
			},
		},
		{
			name: "rollback",
			invoke: func(dependencyErr error, cancelAtWrite bool) (bool, error) {
				callerCtx := context.Background()
				cancel := context.CancelFunc(func() {})
				if cancelAtWrite {
					callerCtx, cancel = context.WithCancel(callerCtx)
				}
				defer cancel()
				current, target, _ := testPublishedV2(t)
				service, err := NewRollbackActivityService(
					activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
						return current, nil
					}),
					publicationReaderFunc(func(context.Context, domain.ActivityID, domain.ActivityPublicationVersion) (domain.ActivityPublication, error) {
						return target, nil
					}),
					publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
						if cancelAtWrite {
							cancel()
						}
						return dependencyErr
					}),
					acceptingLottery(),
					acceptingApproval(),
					ClockFunc(func() time.Time { return target.StartsAt().Add(-time.Minute) }),
					time.Second,
				)
				if err != nil {
					t.Fatalf("new rollback service: %v", err)
				}
				result, operationErr := service.Rollback(callerCtx, RollbackActivityCommand{
					ActivityID:           testActivityID,
					ExpectedStateVersion: current.StateVersion(),
					TargetVersion:        target.Version(),
				})
				return publicationIsZero(result), operationErr
			},
		},
		{
			name: "retire",
			invoke: func(dependencyErr error, cancelAtWrite bool) (bool, error) {
				callerCtx := context.Background()
				cancel := context.CancelFunc(func() {})
				if cancelAtWrite {
					callerCtx, cancel = context.WithCancel(callerCtx)
				}
				defer cancel()
				current, _ := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
				service, err := NewRetireActivityService(
					activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
						return current, nil
					}),
					retirerFunc(func(context.Context, domain.ActivityTransition) error {
						if cancelAtWrite {
							cancel()
						}
						return dependencyErr
					}),
					acceptingApproval(),
					ClockFunc(func() time.Time { return testPublishedAt.Add(20 * time.Minute) }),
					time.Second,
				)
				if err != nil {
					t.Fatalf("new retire service: %v", err)
				}
				result, operationErr := service.Retire(callerCtx, RetireActivityCommand{
					ActivityID:           testActivityID,
					ExpectedStateVersion: current.StateVersion(),
				})
				return result == (domain.Activity{}), operationErr
			},
		},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			for _, class := range []error{ErrRepositoryRetryable, ErrRepositoryFailure} {
				zero, operationErr := operation.invoke(
					WrapRepositoryError(class, errors.New("private ordinary failure")),
					false,
				)
				if !zero || !errors.Is(operationErr, class) {
					t.Fatalf("zero/error = %t/%v, want %v", zero, operationErr, class)
				}
				if _, ok := ActivityCommitReceiptFromError(operationErr); ok {
					t.Fatalf("ordinary %v carried a receipt", class)
				}
			}

			zero, operationErr := operation.invoke(
				WrapRepositoryError(ErrCommitOutcomeUnknown, errors.New("unknown after cancellation")),
				true,
			)
			if !zero || !errors.Is(operationErr, context.Canceled) {
				t.Fatalf("cancel zero/error = %t/%v", zero, operationErr)
			}
			if _, ok := ActivityCommitReceiptFromError(operationErr); ok {
				t.Fatal("cancelled operation carried a receipt")
			}
		})
	}
}

func TestReconcilePublishReceiptUsesExactRootAndFullPublication(t *testing.T) {
	before := testDraft(t)
	transition, err := domain.PlanPublish(
		before,
		testStartsAt,
		testEndsAt,
		testGraphReference(t),
		testStrategyManifest(t),
		"approval/reconciliation-publish",
		testPublishedAt,
	)
	if err != nil {
		t.Fatalf("plan publish: %v", err)
	}
	receipt, err := newActivityCommitReceipt(before, transition)
	if err != nil {
		t.Fatalf("new receipt: %v", err)
	}
	publication, ok := transition.Record()
	if !ok {
		t.Fatal("publish transition has no record")
	}

	assertReconciliation(t, receipt, ObserveCurrentActivity(transition.Next(), publication), ActivityCommitReconciliationCommitted)
	assertReconciliation(t, receipt, ObserveActivityRoot(before), ActivityCommitReconciliationNotCommitted)
	assertReconciliation(t, receipt, ObserveActivityRoot(transition.Next()), ActivityCommitReconciliationIndeterminate)
	assertReconciliation(t, receipt, ActivityCommitObservation{}, ActivityCommitReconciliationIndeterminate)

	competingPublication := restorePublicationWithEvidence(t, publication, "approval/competing-winner")
	assertReconciliation(t, receipt, ObserveCurrentActivity(transition.Next(), competingPublication), ActivityCommitReconciliationNotCommitted)

	wrongIDPublication := restorePublicationWithActivityID(t, publication, testActivityID+1)
	assertReconciliation(t, receipt, ObserveCurrentActivity(transition.Next(), wrongIDPublication), ActivityCommitReconciliationIndeterminate)

	otherDraft, err := domain.NewActivity(testActivityID+1, before.Name().String())
	if err != nil {
		t.Fatalf("other draft: %v", err)
	}
	assertReconciliation(t, receipt, ObserveActivityRoot(otherDraft), ActivityCommitReconciliationIndeterminate)

	advanced, err := domain.PlanPublish(
		transition.Next(),
		testStartsAt,
		testEndsAt.Add(time.Minute),
		testGraphReference(t),
		testStrategyManifest(t),
		"approval/later-publication",
		testPublishedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("plan later publication: %v", err)
	}
	advancedPublication, _ := advanced.Record()
	assertReconciliation(t, receipt, ObserveCurrentActivity(advanced.Next(), advancedPublication), ActivityCommitReconciliationIndeterminate)

	corrupt := receipt
	corrupt.operation = "future"
	assertReconciliation(t, corrupt, ObserveCurrentActivity(transition.Next(), publication), ActivityCommitReconciliationIndeterminate)
}

func TestReconcileReplacementAndRollbackRequireExactCurrentSnapshot(t *testing.T) {
	current, target, currentPublication := testPublishedV2(t)

	t.Run("replacement", func(t *testing.T) {
		transition, err := domain.PlanPublish(
			current,
			testStartsAt,
			testEndsAt.Add(time.Hour),
			testGraphReference(t),
			testStrategyManifest(t),
			"approval/replacement",
			testPublishedAt.Add(time.Minute),
		)
		if err != nil {
			t.Fatalf("plan replacement: %v", err)
		}
		receipt, err := newActivityCommitReceipt(current, transition)
		if err != nil {
			t.Fatalf("new replacement receipt: %v", err)
		}
		record, _ := transition.Record()
		assertReconciliation(t, receipt, ObserveCurrentActivity(transition.Next(), record), ActivityCommitReconciliationCommitted)
		assertReconciliation(t, receipt, ObserveCurrentActivity(current, currentPublication), ActivityCommitReconciliationNotCommitted)
		assertReconciliation(t, receipt, ObserveActivityRoot(current), ActivityCommitReconciliationIndeterminate)
	})

	t.Run("rollback", func(t *testing.T) {
		transition, err := domain.PlanRollback(
			current,
			target,
			true,
			"approval/reconciliation-rollback",
			target.StartsAt().Add(-time.Minute),
		)
		if err != nil {
			t.Fatalf("plan rollback: %v", err)
		}
		receipt, err := newActivityCommitReceipt(current, transition)
		if err != nil {
			t.Fatalf("new rollback receipt: %v", err)
		}
		record, _ := transition.Record()
		assertReconciliation(t, receipt, ObserveCurrentActivity(transition.Next(), record), ActivityCommitReconciliationCommitted)
		assertReconciliation(t, receipt, ObserveCurrentActivity(current, currentPublication), ActivityCommitReconciliationNotCommitted)
		assertReconciliation(t, receipt, ObserveActivityRoot(current), ActivityCommitReconciliationIndeterminate)

		// A domain-valid rollback record may still be invalid for this receipt if
		// it points at the before-image's active version instead of an older one.
		invalidForBefore, err := domain.RestoreActivityPublication(
			record.ActivityID(),
			record.Version(),
			record.SchemaVersion(),
			record.Kind(),
			current.ActivePublicationVersion(),
			record.StartsAt(),
			record.EndsAt(),
			record.PublishedAt(),
			record.GraphReference(),
			record.StrategyRevisionManifest(),
			record.ApprovalEvidenceReference(),
		)
		if err != nil {
			t.Fatalf("restore domain-valid competing rollback: %v", err)
		}
		corruptReceipt := receipt
		corruptReceipt.publication = invalidForBefore
		if !errors.Is(corruptReceipt.Validate(), ErrActivityCommitReceiptInvalid) {
			t.Fatal("rollback receipt accepted rollback-of at the before active version")
		}
		assertReconciliation(t, corruptReceipt, ObserveCurrentActivity(transition.Next(), record), ActivityCommitReconciliationIndeterminate)
	})
}

func TestReconcileRetirementReceiptUsesExactRoot(t *testing.T) {
	before, activePublication := testPublished(t, testStartsAt, testEndsAt, testPublishedAt)
	transition, err := domain.PlanRetire(
		before,
		"approval/reconciliation-retire",
		testPublishedAt.Add(15*time.Minute),
	)
	if err != nil {
		t.Fatalf("plan retirement: %v", err)
	}
	receipt, err := newActivityCommitReceipt(before, transition)
	if err != nil {
		t.Fatalf("new retirement receipt: %v", err)
	}

	assertReconciliation(t, receipt, ObserveActivityRoot(transition.Next()), ActivityCommitReconciliationCommitted)
	assertReconciliation(t, receipt, ObserveCurrentActivity(transition.Next(), activePublication), ActivityCommitReconciliationCommitted)
	assertReconciliation(t, receipt, ObserveActivityRoot(before), ActivityCommitReconciliationNotCommitted)

	competing, err := domain.PlanRetire(
		before,
		"approval/competing-retirement",
		testPublishedAt.Add(16*time.Minute),
	)
	if err != nil {
		t.Fatalf("plan competing retirement: %v", err)
	}
	assertReconciliation(t, receipt, ObserveActivityRoot(competing.Next()), ActivityCommitReconciliationNotCommitted)
	assertReconciliation(t, receipt, ActivityCommitObservation{}, ActivityCommitReconciliationIndeterminate)
}

func TestReceiptConstructionRejectsMismatchedBeforeAndTransition(t *testing.T) {
	before := testDraft(t)
	transition, err := domain.PlanPublish(
		before,
		testStartsAt,
		testEndsAt,
		testGraphReference(t),
		testStrategyManifest(t),
		"approval/receipt-construction",
		testPublishedAt,
	)
	if err != nil {
		t.Fatalf("plan publish: %v", err)
	}
	other, err := domain.NewActivity(testActivityID+1, before.Name().String())
	if err != nil {
		t.Fatalf("new other draft: %v", err)
	}
	if receipt, err := newActivityCommitReceipt(other, transition); !errors.Is(err, ErrActivityCommitReceiptInvalid) ||
		receipt.Operation() != "" {
		t.Fatalf("receipt/error = %#v/%v", receipt, err)
	}
}

func requireCommitReceipt(
	t *testing.T,
	err error,
	wantOperation ActivityCommitOperation,
) ActivityCommitReceipt {
	t.Helper()
	receipt, ok := ActivityCommitReceiptFromError(err)
	if !ok || receipt.Validate() != nil || receipt.Operation() != wantOperation {
		t.Fatalf("receipt = %#v/%t, want valid %q", receipt, ok, wantOperation)
	}
	return receipt
}

func assertLowDisclosureCommitError(t *testing.T, err error, secret error) {
	t.Helper()
	if err.Error() != ErrCommitOutcomeUnknown.Error() || strings.Contains(err.Error(), "credential") ||
		errors.Is(err, secret) || errors.Unwrap(err) != nil {
		t.Fatalf("commit error leaked: %q", err)
	}
	operationError, ok := err.(*ActivityOperationError)
	if !ok || !errors.Is(operationError.Cause(), ErrCommitOutcomeUnknown) {
		t.Fatalf("trusted cause = %#v", operationError.Cause())
	}
}

func testUnknownPublishReceipt(t *testing.T) (ActivityCommitReceipt, error) {
	t.Helper()
	service, err := NewPublishActivityService(
		activityReaderFunc(func(context.Context, domain.ActivityID) (domain.Activity, error) {
			return testDraft(t), nil
		}),
		publicationWriterFunc(func(context.Context, domain.ActivityTransition) error {
			return WrapRepositoryError(ErrCommitOutcomeUnknown, errors.New("private unknown"))
		}),
		acceptingLottery(),
		acceptingApproval(),
		ClockFunc(func() time.Time { return testPublishedAt }),
		time.Second,
	)
	if err != nil {
		t.Fatalf("new publish service: %v", err)
	}
	result, operationErr := service.Publish(context.Background(), testPublishCommand(t))
	if !publicationIsZero(result) {
		t.Fatalf("unknown commit returned publication %#v", result)
	}
	return requireCommitReceipt(t, operationErr, ActivityCommitOperationPublish), operationErr
}

func assertReconciliation(
	t *testing.T,
	receipt ActivityCommitReceipt,
	observation ActivityCommitObservation,
	want ActivityCommitReconciliation,
) {
	t.Helper()
	if got := ReconcileActivityCommit(receipt, observation); got != want {
		t.Fatalf("reconciliation = %q, want %q", got, want)
	}
}

func restorePublicationWithEvidence(
	t *testing.T,
	publication domain.ActivityPublication,
	evidence domain.EvidenceReference,
) domain.ActivityPublication {
	t.Helper()
	rollbackOf, _ := publication.RollbackOf()
	result, err := domain.RestoreActivityPublication(
		publication.ActivityID(),
		publication.Version(),
		publication.SchemaVersion(),
		publication.Kind(),
		rollbackOf,
		publication.StartsAt(),
		publication.EndsAt(),
		publication.PublishedAt(),
		publication.GraphReference(),
		publication.StrategyRevisionManifest(),
		evidence,
	)
	if err != nil {
		t.Fatalf("restore publication with evidence: %v", err)
	}
	return result
}

func restorePublicationWithActivityID(
	t *testing.T,
	publication domain.ActivityPublication,
	activityID domain.ActivityID,
) domain.ActivityPublication {
	t.Helper()
	rollbackOf, _ := publication.RollbackOf()
	result, err := domain.RestoreActivityPublication(
		activityID,
		publication.Version(),
		publication.SchemaVersion(),
		publication.Kind(),
		rollbackOf,
		publication.StartsAt(),
		publication.EndsAt(),
		publication.PublishedAt(),
		publication.GraphReference(),
		publication.StrategyRevisionManifest(),
		publication.ApprovalEvidenceReference(),
	)
	if err != nil {
		t.Fatalf("restore publication with activity id: %v", err)
	}
	return result
}
