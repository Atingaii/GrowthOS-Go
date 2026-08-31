package application

import (
	"slices"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// ActivityCommitOperation identifies the one high-risk command represented by
// an ActivityCommitReceipt. It is deliberately narrower than repository
// transaction names so a caller cannot reconcile one operation as another.
type ActivityCommitOperation string

const (
	ActivityCommitOperationPublish  ActivityCommitOperation = "publish"
	ActivityCommitOperationRollback ActivityCommitOperation = "rollback"
	ActivityCommitOperationRetire   ActivityCommitOperation = "retire"
)

// ActivityCommitReconciliation is the closed result of comparing a trusted
// commit receipt with one exact read-back observation.
type ActivityCommitReconciliation string

const (
	// ActivityCommitReconciliationCommitted means the exact after-image, and for
	// publication commands the complete immutable publication, were observed.
	ActivityCommitReconciliationCommitted ActivityCommitReconciliation = "committed"
	// ActivityCommitReconciliationNotCommitted means the exact before-image or a
	// different valid winner at the same next state version was observed.
	ActivityCommitReconciliationNotCommitted ActivityCommitReconciliation = "not_committed"
	// ActivityCommitReconciliationIndeterminate is the fail-closed result for a
	// missing, invalid, mismatched, or subsequently advanced observation.
	ActivityCommitReconciliationIndeterminate ActivityCommitReconciliation = "indeterminate"
)

// ActivityCommitReceipt is an immutable application-owned description of one
// exact write attempt whose COMMIT acknowledgement was lost. Publication
// attempts retain the full append-only record; retirement retains the complete
// next root, including its server-controlled instant and approval evidence.
//
// The zero value is invalid. Receipts can only be obtained from the explicit
// ActivityOperationError accessor after an ErrCommitOutcomeUnknown result.
type ActivityCommitReceipt struct {
	operation   ActivityCommitOperation
	before      domain.Activity
	after       domain.Activity
	publication domain.ActivityPublication
}

// Operation reports publish, rollback, or retire.
func (receipt ActivityCommitReceipt) Operation() ActivityCommitOperation {
	return receipt.operation
}

// Before returns the exact root read before the write attempt.
func (receipt ActivityCommitReceipt) Before() domain.Activity { return receipt.before }

// After returns the complete root that the write attempt tried to persist.
func (receipt ActivityCommitReceipt) After() domain.Activity { return receipt.after }

// Publication returns a defensive copy of the full immutable record for a
// publish or rollback attempt. Retirement returns the zero record and false.
func (receipt ActivityCommitReceipt) Publication() (domain.ActivityPublication, bool) {
	if receipt.operation == ActivityCommitOperationRetire || publicationIsZero(receipt.publication) {
		return domain.ActivityPublication{}, false
	}
	publication, err := cloneActivityPublication(receipt.publication)
	if err != nil {
		return domain.ActivityPublication{}, false
	}
	return publication, true
}

// Validate verifies the closed receipt shape without rendering any contained
// publication, evidence, or foreign-reference value.
func (receipt ActivityCommitReceipt) Validate() error {
	if !activityCommitReceiptIsValid(receipt) {
		return ErrActivityCommitReceiptInvalid
	}
	return nil
}

// ActivityCommitObservation wraps either the exact current root/publication
// snapshot used after publish or rollback, or the exact root used after
// retirement. Its fields are private so callers cannot forge its observation
// mode. Invalid domain values remain invalid and reconcile as indeterminate.
type ActivityCommitObservation struct {
	root            domain.Activity
	publication     domain.ActivityPublication
	currentSnapshot bool
}

// ObserveCurrentActivity forms a current-snapshot observation. Draft roots
// require a zero publication; published and retired roots require their exact
// active publication.
func ObserveCurrentActivity(
	root domain.Activity,
	publication domain.ActivityPublication,
) ActivityCommitObservation {
	return ActivityCommitObservation{
		root:            root,
		publication:     publication,
		currentSnapshot: true,
	}
}

// ObserveActivityRoot forms a root-only observation for retirement read-back.
func ObserveActivityRoot(root domain.Activity) ActivityCommitObservation {
	return ActivityCommitObservation{root: root}
}

// ReconcileActivityCommit compares one trusted receipt with one exact
// observation. It performs no I/O and never recommends blind replay. Bad or
// incomplete input always returns indeterminate.
func ReconcileActivityCommit(
	receipt ActivityCommitReceipt,
	observation ActivityCommitObservation,
) ActivityCommitReconciliation {
	if !activityCommitReceiptIsValid(receipt) || !activityCommitObservationIsValid(observation) {
		return ActivityCommitReconciliationIndeterminate
	}

	observed := observation.root
	if observed.ID() != receipt.before.ID() || observed.Name() != receipt.before.Name() {
		return ActivityCommitReconciliationIndeterminate
	}

	if sameActivity(observed, receipt.after) {
		if receipt.operation == ActivityCommitOperationRetire {
			return ActivityCommitReconciliationCommitted
		}
		if !observation.currentSnapshot {
			return ActivityCommitReconciliationIndeterminate
		}
		if sameActivityPublication(observation.publication, receipt.publication) {
			return ActivityCommitReconciliationCommitted
		}
		// The root points at the attempted next identity but a different valid
		// immutable record occupies it, so another same-generation writer won.
		return ActivityCommitReconciliationNotCommitted
	}

	if sameActivity(observed, receipt.before) {
		if receipt.operation == ActivityCommitOperationRetire ||
			receipt.before.Lifecycle() == domain.ActivityLifecycleDraft ||
			observation.currentSnapshot {
			return ActivityCommitReconciliationNotCommitted
		}
		return ActivityCommitReconciliationIndeterminate
	}

	// A different valid after-image at exactly the attempted next generation is
	// proof that this CAS did not win. A later generation cannot prove whether
	// this attempt committed before another command advanced it.
	if observed.StateVersion() == receipt.after.StateVersion() {
		if receipt.operation != ActivityCommitOperationRetire && !observation.currentSnapshot {
			return ActivityCommitReconciliationIndeterminate
		}
		return ActivityCommitReconciliationNotCommitted
	}
	return ActivityCommitReconciliationIndeterminate
}

func newActivityCommitReceipt(
	before domain.Activity,
	transition domain.ActivityTransition,
) (ActivityCommitReceipt, error) {
	if err := before.Validate(); err != nil || transition.Validate() != nil ||
		before.Lifecycle() != transition.ExpectedLifecycle() ||
		before.StateVersion() != transition.ExpectedStateVersion() ||
		before.ActivePublicationVersion() != transition.ExpectedActivePublicationVersion() {
		return ActivityCommitReceipt{}, ErrActivityCommitReceiptInvalid
	}

	receipt := ActivityCommitReceipt{
		before: before,
		after:  transition.Next(),
	}
	if publication, ok := transition.Record(); ok {
		receipt.publication = publication
		switch publication.Kind() {
		case domain.ActivityPublicationKindRelease:
			receipt.operation = ActivityCommitOperationPublish
		case domain.ActivityPublicationKindRollback:
			receipt.operation = ActivityCommitOperationRollback
		default:
			return ActivityCommitReceipt{}, ErrActivityCommitReceiptInvalid
		}
	} else {
		receipt.operation = ActivityCommitOperationRetire
	}
	if !activityCommitReceiptIsValid(receipt) {
		return ActivityCommitReceipt{}, ErrActivityCommitReceiptInvalid
	}
	return cloneActivityCommitReceipt(receipt)
}

func activityCommitReceiptIsValid(receipt ActivityCommitReceipt) bool {
	if receipt.before.Validate() != nil || receipt.after.Validate() != nil ||
		receipt.before.ID() != receipt.after.ID() ||
		receipt.before.Name() != receipt.after.Name() ||
		receipt.before.StateVersion() == domain.ActivityStateVersion(^uint64(0)) ||
		receipt.after.StateVersion() != receipt.before.StateVersion()+1 {
		return false
	}

	switch receipt.operation {
	case ActivityCommitOperationPublish, ActivityCommitOperationRollback:
		if receipt.after.Lifecycle() != domain.ActivityLifecyclePublished ||
			receipt.after.ActivePublicationVersion() != receipt.before.ActivePublicationVersion()+1 ||
			receipt.publication.Validate() != nil ||
			receipt.publication.ActivityID() != receipt.after.ID() ||
			receipt.publication.Version() != receipt.after.ActivePublicationVersion() {
			return false
		}
		if receipt.operation == ActivityCommitOperationPublish {
			return (receipt.before.Lifecycle() == domain.ActivityLifecycleDraft ||
				receipt.before.Lifecycle() == domain.ActivityLifecyclePublished) &&
				receipt.publication.Kind() == domain.ActivityPublicationKindRelease
		}
		rollbackOf, rollback := receipt.publication.RollbackOf()
		return receipt.before.Lifecycle() == domain.ActivityLifecyclePublished &&
			receipt.publication.Kind() == domain.ActivityPublicationKindRollback &&
			rollback && rollbackOf < receipt.before.ActivePublicationVersion()
	case ActivityCommitOperationRetire:
		return receipt.before.Lifecycle() == domain.ActivityLifecyclePublished &&
			receipt.after.Lifecycle() == domain.ActivityLifecycleRetired &&
			receipt.after.ActivePublicationVersion() == receipt.before.ActivePublicationVersion() &&
			publicationIsZero(receipt.publication)
	default:
		return false
	}
}

func activityCommitObservationIsValid(observation ActivityCommitObservation) bool {
	if observation.root.Validate() != nil {
		return false
	}
	if !observation.currentSnapshot {
		return publicationIsZero(observation.publication)
	}
	if observation.root.Lifecycle() == domain.ActivityLifecycleDraft {
		return publicationIsZero(observation.publication)
	}
	return observation.publication.Validate() == nil &&
		observation.publication.ActivityID() == observation.root.ID() &&
		observation.publication.Version() == observation.root.ActivePublicationVersion()
}

func cloneActivityCommitReceipt(receipt ActivityCommitReceipt) (ActivityCommitReceipt, error) {
	clone := receipt
	if !publicationIsZero(receipt.publication) {
		publication, err := cloneActivityPublication(receipt.publication)
		if err != nil {
			return ActivityCommitReceipt{}, ErrActivityCommitReceiptInvalid
		}
		clone.publication = publication
	}
	if !activityCommitReceiptIsValid(clone) {
		return ActivityCommitReceipt{}, ErrActivityCommitReceiptInvalid
	}
	return clone, nil
}

func cloneActivityPublication(
	publication domain.ActivityPublication,
) (domain.ActivityPublication, error) {
	rollbackOf, _ := publication.RollbackOf()
	return domain.RestoreActivityPublication(
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
		publication.ApprovalEvidenceReference(),
	)
}

func sameActivity(left, right domain.Activity) bool {
	leftRetiredAt, leftHasRetiredAt := left.RetiredAt()
	rightRetiredAt, rightHasRetiredAt := right.RetiredAt()
	leftRetirement, leftHasRetirement := left.RetirementReference()
	rightRetirement, rightHasRetirement := right.RetirementReference()
	return left.ID() == right.ID() &&
		left.Name() == right.Name() &&
		left.Lifecycle() == right.Lifecycle() &&
		left.StateVersion() == right.StateVersion() &&
		left.ActivePublicationVersion() == right.ActivePublicationVersion() &&
		leftHasRetiredAt == rightHasRetiredAt &&
		leftRetiredAt == rightRetiredAt &&
		leftHasRetirement == rightHasRetirement &&
		leftRetirement == rightRetirement
}

func sameActivityPublication(left, right domain.ActivityPublication) bool {
	leftRollback, leftIsRollback := left.RollbackOf()
	rightRollback, rightIsRollback := right.RollbackOf()
	return left.ActivityID() == right.ActivityID() &&
		left.Version() == right.Version() &&
		left.SchemaVersion() == right.SchemaVersion() &&
		left.Kind() == right.Kind() &&
		leftIsRollback == rightIsRollback &&
		leftRollback == rightRollback &&
		left.StartsAt() == right.StartsAt() &&
		left.EndsAt() == right.EndsAt() &&
		left.PublishedAt() == right.PublishedAt() &&
		left.GraphReference() == right.GraphReference() &&
		slices.Equal(left.StrategyRevisionManifest(), right.StrategyRevisionManifest()) &&
		left.ApprovalEvidenceReference() == right.ApprovalEvidenceReference()
}
