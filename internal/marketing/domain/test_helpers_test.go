package domain

import (
	"testing"
	"time"
)

func testMarketingInstant() time.Time {
	return time.Date(2026, time.August, 30, 12, 0, 0, 123456000, time.UTC)
}

func mustTestActivity(t *testing.T, id ActivityID) Activity {
	t.Helper()
	activity, err := NewActivity(id, " Summer Growth ")
	if err != nil {
		t.Fatalf("NewActivity: %v", err)
	}
	return activity
}

func mustTestPublishedActivity(
	t *testing.T,
	id ActivityID,
	version ActivityPublicationVersion,
) Activity {
	t.Helper()
	activity, err := RestoreActivity(
		id,
		"Summer Growth",
		ActivityLifecyclePublished,
		ActivityStateVersion(version),
		version,
		time.Time{},
		"",
	)
	if err != nil {
		t.Fatalf("RestoreActivity: %v", err)
	}
	return activity
}

func mustTestGraphReference(t *testing.T, id LotteryGraphID, revision string) LotteryGraphReference {
	t.Helper()
	reference, err := NewLotteryGraphReference(id, revision)
	if err != nil {
		t.Fatalf("NewLotteryGraphReference: %v", err)
	}
	return reference
}

func mustTestStrategyReference(
	t *testing.T,
	id LotteryStrategyID,
	revision string,
) LotteryStrategyRevisionReference {
	t.Helper()
	reference, err := NewLotteryStrategyRevisionReference(id, revision)
	if err != nil {
		t.Fatalf("NewLotteryStrategyRevisionReference: %v", err)
	}
	return reference
}

func testStrategyManifest(t *testing.T) []LotteryStrategyRevisionReference {
	t.Helper()
	return []LotteryStrategyRevisionReference{
		mustTestStrategyReference(t, 22, "strategy-22:r4"),
		mustTestStrategyReference(t, 11, "strategy-11:r7"),
	}
}

func mustTestEvidence(t *testing.T, value string) EvidenceReference {
	t.Helper()
	reference, err := NewEvidenceReference(value)
	if err != nil {
		t.Fatalf("NewEvidenceReference: %v", err)
	}
	return reference
}

func mustTestReleasePublication(
	t *testing.T,
	activityID ActivityID,
	version ActivityPublicationVersion,
	startsAt time.Time,
	endsAt time.Time,
	publishedAt time.Time,
) ActivityPublication {
	t.Helper()
	publication, err := RestoreActivityPublication(
		activityID,
		version,
		ActivityPublicationSchemaVersionV1,
		ActivityPublicationKindRelease,
		0,
		startsAt,
		endsAt,
		publishedAt,
		mustTestGraphReference(t, 9, "graph:r3"),
		testStrategyManifest(t),
		mustTestEvidence(t, "approval/change-17"),
	)
	if err != nil {
		t.Fatalf("RestoreActivityPublication: %v", err)
	}
	return publication
}

func assertZeroTransition(t *testing.T, transition ActivityTransition) {
	t.Helper()
	if transition.expectedLifecycle != "" ||
		transition.expectedStateVersion != 0 ||
		transition.expectedActivePublicationVersion != 0 ||
		transition.next != (Activity{}) ||
		!transition.record.isZero() ||
		transition.appendsPublication {
		t.Fatalf("expected zero transition, got %#v", transition)
	}
}

func assertZeroGateDecision(t *testing.T, decision ActivityGateDecision) {
	t.Helper()
	if decision != (ActivityGateDecision{}) {
		t.Fatalf("expected zero gate decision, got %#v", decision)
	}
}
