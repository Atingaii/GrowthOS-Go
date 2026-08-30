package application

import (
	"context"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// ActivityDraftCreator persists one new draft root. It implies no update,
// upsert, delete, or publication semantics.
type ActivityDraftCreator interface {
	CreateDraft(ctx context.Context, activity domain.Activity) error
}

// ActivityReader restores one Activity root without guessing or loading an
// active publication.
type ActivityReader interface {
	FindActivityByID(ctx context.Context, id domain.ActivityID) (domain.Activity, error)
}

// ActivityCurrentReader restores one root and its exact active publication in
// a single read-only REPEATABLE READ snapshot. Draft returns a valid Activity,
// the zero publication, and nil. Published and retired roots must return their
// exact active publication; implementations never select max(version).
type ActivityCurrentReader interface {
	FindCurrentActivity(
		ctx context.Context,
		id domain.ActivityID,
	) (domain.Activity, domain.ActivityPublication, error)
}

// ActivityPublicationReader restores exactly one append-only historical
// publication. It never falls back to current, latest, or another Activity.
type ActivityPublicationReader interface {
	FindPublicationByIdentity(
		ctx context.Context,
		activityID domain.ActivityID,
		version domain.ActivityPublicationVersion,
	) (domain.ActivityPublication, error)
}

// ActivityPublicationWriter atomically inserts the publication and all of its
// Strategy bindings, then compares and swaps the Activity root. It must leave
// no orphan rows when the CAS loses.
type ActivityPublicationWriter interface {
	CompareAndSwapPublication(ctx context.Context, transition domain.ActivityTransition) error
}

// ActivityRetirer compares and swaps the terminal root transition without
// modifying or deleting any historical publication.
type ActivityRetirer interface {
	CompareAndSwapRetirement(ctx context.Context, transition domain.ActivityTransition) error
}

// Clock supplies one server-owned business instant per publish, rollback,
// retirement, or resolution operation.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function to Clock. A typed-nil ClockFunc is rejected by
// service validation.
type ClockFunc func() time.Time

// Now returns the function result.
func (function ClockFunc) Now() time.Time { return function() }

// ApprovalVerifier consumes exact immutable candidates. It returns only a
// bounded evidence reference; approval is neither identity nor authorization.
type ApprovalVerifier interface {
	VerifyPublication(
		ctx context.Context,
		candidate ActivityPublicationCandidate,
	) (domain.EvidenceReference, error)
	VerifyRetirement(
		ctx context.Context,
		candidate ActivityRetirementCandidate,
	) (domain.EvidenceReference, error)
}

// LotteryVerifier proves that an exact graph exists, is valid, and has exactly
// the same unique terminal Strategy IDs as the exact Strategy snapshot
// manifest. It must never query latest revisions.
type LotteryVerifier interface {
	VerifyPublication(ctx context.Context, candidate ActivityPublicationCandidate) error
}
