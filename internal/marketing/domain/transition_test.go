package domain

import (
	"errors"
	"testing"
	"time"
)

func TestPlanPublishAppendsMonotonicReleaseFromDraftAndPublished(t *testing.T) {
	draft := mustTestActivity(t, 41)
	publishedAt := testMarketingInstant().In(time.FixedZone("UTC+8", 8*60*60)).Add(999 * time.Nanosecond)
	start := testMarketingInstant().Add(time.Hour + 999*time.Nanosecond)
	end := start.Add(24 * time.Hour)
	manifest := testStrategyManifest(t)
	first, err := PlanPublish(
		draft,
		start,
		end,
		mustTestGraphReference(t, 9, "graph:r1"),
		manifest,
		mustTestEvidence(t, "approval/release-1"),
		publishedAt,
	)
	if err != nil {
		t.Fatalf("PlanPublish draft: %v", err)
	}
	assertPublishTransitionCAS(t, first, ActivityLifecycleDraft, 0, 0, 1)
	record, ok := first.Record()
	if !ok || record.Version() != 1 || record.Kind() != ActivityPublicationKindRelease ||
		record.PublishedAt() != canonicalUTCInstant(publishedAt) ||
		record.StartsAt() != canonicalUTCInstant(start) ||
		record.EndsAt() != canonicalUTCInstant(end) {
		t.Fatalf("first record = %#v/%v", record, ok)
	}
	if rollbackOf, rollback := record.RollbackOf(); rollback || rollbackOf != 0 {
		t.Fatalf("first rollback-of = %d/%v", rollbackOf, rollback)
	}
	manifest[0] = mustTestStrategyReference(t, 999, "mutated:r1")
	if record.StrategyRevisionManifest()[1].StrategyID() != 22 {
		t.Fatal("transition record aliases PlanPublish input")
	}

	second, err := PlanPublish(
		first.Next(),
		start.Add(time.Hour),
		end.Add(time.Hour),
		mustTestGraphReference(t, 10, "graph:r2"),
		[]LotteryStrategyRevisionReference{mustTestStrategyReference(t, 33, "strategy:r2")},
		mustTestEvidence(t, "approval/release-2"),
		testMarketingInstant().Add(5*time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanPublish published: %v", err)
	}
	assertPublishTransitionCAS(t, second, ActivityLifecyclePublished, 1, 1, 2)
	secondRecord, _ := second.Record()
	if secondRecord.Version() != 2 || secondRecord.GraphReference().ID() != 10 {
		t.Fatalf("second record version/graph = %d/%d", secondRecord.Version(), secondRecord.GraphReference().ID())
	}
}

func TestPlanRollbackAppendsNewVersionAndCopiesExactTargetContent(t *testing.T) {
	draft := mustTestActivity(t, 7)
	now := testMarketingInstant()
	first, err := PlanPublish(
		draft,
		now.Add(time.Hour),
		now.Add(48*time.Hour),
		mustTestGraphReference(t, 11, "graph:old"),
		testStrategyManifest(t),
		mustTestEvidence(t, "approval/release-1"),
		now,
	)
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	target, _ := first.Record()
	second, err := PlanPublish(
		first.Next(),
		now,
		now.Add(72*time.Hour),
		mustTestGraphReference(t, 12, "graph:new"),
		[]LotteryStrategyRevisionReference{mustTestStrategyReference(t, 44, "strategy:new")},
		mustTestEvidence(t, "approval/release-2"),
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	rollbackAt := now.Add(2*time.Minute + 999*time.Nanosecond).In(time.FixedZone("UTC+8", 8*60*60))
	rollback, err := PlanRollback(
		second.Next(),
		target,
		true,
		mustTestEvidence(t, "approval/rollback-3"),
		rollbackAt,
	)
	if err != nil {
		t.Fatalf("PlanRollback: %v", err)
	}
	assertPublishTransitionCAS(t, rollback, ActivityLifecyclePublished, 2, 2, 3)
	record, ok := rollback.Record()
	if !ok || record.Version() != 3 || record.Kind() != ActivityPublicationKindRollback ||
		record.SchemaVersion() != ActivityPublicationSchemaVersionV1 {
		t.Fatalf("rollback record = %#v/%v", record, ok)
	}
	if rollbackOf, isRollback := record.RollbackOf(); !isRollback || rollbackOf != target.Version() {
		t.Fatalf("rollback-of = %d/%v", rollbackOf, isRollback)
	}
	if record.StartsAt() != target.StartsAt() || record.EndsAt() != target.EndsAt() ||
		record.GraphReference() != target.GraphReference() ||
		record.PublishedAt() != canonicalUTCInstant(rollbackAt) ||
		record.PublishedAt() == target.PublishedAt() ||
		record.ApprovalEvidenceReference() != "approval/rollback-3" ||
		record.ApprovalEvidenceReference() == target.ApprovalEvidenceReference() {
		t.Fatalf("rollback did not copy exact content with new metadata: %#v", record)
	}
	assertStrategyManifestsEqual(t, record.StrategyRevisionManifest(), target.StrategyRevisionManifest())
}

func TestPlanRetireIsTerminalAndKeepsLastActivePublication(t *testing.T) {
	now := testMarketingInstant()
	published := mustTestPublishedActivity(t, 7, 4)
	retiredAt := now.In(time.FixedZone("UTC+8", 8*60*60)).Add(999 * time.Nanosecond)
	transition, err := PlanRetire(
		published,
		mustTestEvidence(t, "retirement/incident-9"),
		retiredAt,
	)
	if err != nil {
		t.Fatalf("PlanRetire: %v", err)
	}
	if transition.ExpectedLifecycle() != ActivityLifecyclePublished ||
		transition.ExpectedStateVersion() != 4 ||
		transition.ExpectedActivePublicationVersion() != 4 ||
		transition.AppendsPublication() {
		t.Fatalf("retire CAS/append = %#v", transition)
	}
	if record, ok := transition.Record(); ok || !record.isZero() {
		t.Fatalf("retire record = %#v/%v", record, ok)
	}
	next := transition.Next()
	if next.Lifecycle() != ActivityLifecycleRetired || next.StateVersion() != 5 ||
		next.ActivePublicationVersion() != 4 {
		t.Fatalf("retired state = %q/%d/%d", next.Lifecycle(), next.StateVersion(), next.ActivePublicationVersion())
	}
	if got, ok := next.RetiredAt(); !ok || got != canonicalUTCInstant(retiredAt) {
		t.Fatalf("retired-at = %v/%v", got, ok)
	}
	if got, ok := next.RetirementReference(); !ok || got != "retirement/incident-9" {
		t.Fatalf("retirement reference = %q/%v", got, ok)
	}
	if err := transition.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestTransitionPlansRejectIllegalLifecycleMovesWithZeroPlan(t *testing.T) {
	now := testMarketingInstant()
	draft := mustTestActivity(t, 1)
	target := mustTestReleasePublication(t, 1, 1, now, now.Add(time.Hour), now)
	published := mustTestPublishedActivity(t, 1, 1)
	retiredTransition, err := PlanRetire(published, mustTestEvidence(t, "retirement/1"), now)
	if err != nil {
		t.Fatalf("prepare retired: %v", err)
	}
	retired := retiredTransition.Next()
	tests := []struct {
		name string
		plan func() (ActivityTransition, error)
	}{
		{name: "rollback draft", plan: func() (ActivityTransition, error) {
			return PlanRollback(draft, target, true, mustTestEvidence(t, "approval/rollback"), now)
		}},
		{name: "retire draft", plan: func() (ActivityTransition, error) {
			return PlanRetire(draft, mustTestEvidence(t, "retirement/2"), now)
		}},
		{name: "publish retired", plan: func() (ActivityTransition, error) {
			return PlanPublish(retired, now, now.Add(time.Hour), mustTestGraphReference(t, 1, "g1"), testStrategyManifest(t), mustTestEvidence(t, "approval/release"), now)
		}},
		{name: "rollback retired", plan: func() (ActivityTransition, error) {
			return PlanRollback(retired, target, true, mustTestEvidence(t, "approval/rollback"), now)
		}},
		{name: "retire retired", plan: func() (ActivityTransition, error) {
			return PlanRetire(retired, mustTestEvidence(t, "retirement/3"), now)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition, err := test.plan()
			if !errors.Is(err, ErrActivityLifecycleTransitionInvalid) {
				t.Fatalf("plan error = %v", err)
			}
			assertZeroTransition(t, transition)
		})
	}
}

func TestPlanPublishRejectsInvalidContentExpiredWindowAndOverflow(t *testing.T) {
	now := testMarketingInstant()
	draft := mustTestActivity(t, 1)
	graph := mustTestGraphReference(t, 1, "graph:r1")
	manifest := testStrategyManifest(t)
	evidence := mustTestEvidence(t, "approval/release")
	tests := []struct {
		name     string
		current  Activity
		start    time.Time
		end      time.Time
		graph    LotteryGraphReference
		manifest []LotteryStrategyRevisionReference
		evidence EvidenceReference
		at       time.Time
		want     error
	}{
		{name: "empty window", current: draft, start: now, end: now, graph: graph, manifest: manifest, evidence: evidence, at: now.Add(-time.Second), want: ErrActivityPublicationWindowInvalid},
		{name: "precision collapses window", current: draft, start: now, end: now.Add(time.Nanosecond), graph: graph, manifest: manifest, evidence: evidence, at: now.Add(-time.Second), want: ErrActivityPublicationWindowInvalid},
		{name: "publish at end", current: draft, start: now, end: now.Add(time.Hour), graph: graph, manifest: manifest, evidence: evidence, at: now.Add(time.Hour), want: ErrActivityPublicationExpired},
		{name: "publish after end", current: draft, start: now, end: now.Add(time.Hour), graph: graph, manifest: manifest, evidence: evidence, at: now.Add(2 * time.Hour), want: ErrActivityPublicationExpired},
		{name: "missing published at", current: draft, start: now, end: now.Add(time.Hour), graph: graph, manifest: manifest, evidence: evidence, want: ErrActivityPublicationWindowInvalid},
		{name: "invalid graph", current: draft, start: now, end: now.Add(time.Hour), manifest: manifest, evidence: evidence, at: now, want: ErrLotteryGraphReferenceInvalid},
		{name: "empty manifest", current: draft, start: now, end: now.Add(time.Hour), graph: graph, evidence: evidence, at: now, want: ErrStrategyRevisionManifestInvalid},
		{name: "duplicate manifest", current: draft, start: now, end: now.Add(time.Hour), graph: graph, manifest: []LotteryStrategyRevisionReference{manifest[0], manifest[0]}, evidence: evidence, at: now, want: ErrStrategyRevisionManifestInvalid},
		{name: "manifest limit", current: draft, start: now, end: now.Add(time.Hour), graph: graph, manifest: strategyManifestWithCount(MaxStrategyRevisionManifestEntries + 1), evidence: evidence, at: now, want: ErrStrategyRevisionManifestLimitExceeded},
		{name: "invalid approval", current: draft, start: now, end: now.Add(time.Hour), graph: graph, manifest: manifest, evidence: "bad ref", at: now, want: ErrEvidenceReferenceInvalid},
		{name: "overflow", current: mustTestPublishedActivity(t, 1, maxActivityVersion), start: now, end: now.Add(time.Hour), graph: graph, manifest: manifest, evidence: evidence, at: now, want: ErrActivityVersionOverflow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition, err := PlanPublish(
				test.current,
				test.start,
				test.end,
				test.graph,
				test.manifest,
				test.evidence,
				test.at,
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("PlanPublish() error = %v, want %v", err, test.want)
			}
			assertZeroTransition(t, transition)
		})
	}
}

func TestPlanRollbackRejectsUnprovenWrongCurrentOrExpiredTarget(t *testing.T) {
	now := testMarketingInstant()
	current := mustTestPublishedActivity(t, 1, 2)
	target := mustTestReleasePublication(t, 1, 1, now, now.Add(time.Hour), now)
	evidence := mustTestEvidence(t, "approval/rollback")
	wrongActivity := target.clone()
	wrongActivity.activityID = 2
	currentTarget := mustTestReleasePublication(t, 1, 2, now, now.Add(time.Hour), now)
	futureTarget := mustTestReleasePublication(t, 1, 3, now, now.Add(time.Hour), now)
	invalidTarget := target.clone()
	invalidTarget.schemaVersion = 0
	tests := []struct {
		name      string
		current   Activity
		target    ActivityPublication
		published bool
		evidence  EvidenceReference
		at        time.Time
		want      error
	}{
		{name: "invalid target", current: current, target: invalidTarget, published: true, evidence: evidence, at: now, want: ErrActivityRollbackTargetInvalid},
		{name: "wrong Activity", current: current, target: wrongActivity, published: true, evidence: evidence, at: now, want: ErrActivityRollbackTargetInvalid},
		{name: "current target", current: current, target: currentTarget, published: true, evidence: evidence, at: now, want: ErrActivityRollbackTargetInvalid},
		{name: "future target", current: current, target: futureTarget, published: true, evidence: evidence, at: now, want: ErrActivityRollbackTargetInvalid},
		{name: "not historically published", current: current, target: target, evidence: evidence, at: now, want: ErrActivityRollbackTargetNotPublished},
		{name: "at exclusive end", current: current, target: target, published: true, evidence: evidence, at: target.EndsAt(), want: ErrActivityPublicationExpired},
		{name: "after end", current: current, target: target, published: true, evidence: evidence, at: target.EndsAt().Add(time.Second), want: ErrActivityPublicationExpired},
		{name: "missing published at", current: current, target: target, published: true, evidence: evidence, want: ErrActivityRollbackTargetInvalid},
		{name: "invalid approval", current: current, target: target, published: true, evidence: "bad ref", at: now, want: ErrEvidenceReferenceInvalid},
		{name: "version overflow", current: mustTestPublishedActivity(t, 1, maxActivityVersion), target: target, published: true, evidence: evidence, at: now, want: ErrActivityVersionOverflow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition, err := PlanRollback(
				test.current,
				test.target,
				test.published,
				test.evidence,
				test.at,
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("PlanRollback() error = %v, want %v", err, test.want)
			}
			assertZeroTransition(t, transition)
		})
	}
}

func TestPlanRetireRejectsBadEvidenceTimeAndOverflow(t *testing.T) {
	now := testMarketingInstant()
	tests := []struct {
		name      string
		current   Activity
		evidence  EvidenceReference
		retiredAt time.Time
		want      error
	}{
		{name: "evidence", current: mustTestPublishedActivity(t, 1, 1), evidence: "bad ref", retiredAt: now, want: ErrEvidenceReferenceInvalid},
		{name: "time", current: mustTestPublishedActivity(t, 1, 1), evidence: mustTestEvidence(t, "retirement/1"), want: ErrActivityLifecycleTransitionInvalid},
		{name: "overflow", current: mustTestPublishedActivity(t, 1, maxActivityVersion), evidence: mustTestEvidence(t, "retirement/1"), retiredAt: now, want: ErrActivityVersionOverflow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition, err := PlanRetire(test.current, test.evidence, test.retiredAt)
			if !errors.Is(err, test.want) {
				t.Fatalf("PlanRetire() error = %v, want %v", err, test.want)
			}
			assertZeroTransition(t, transition)
		})
	}
}

func TestActivityTransitionRejectsForgedPartialPlans(t *testing.T) {
	now := testMarketingInstant()
	valid, err := PlanPublish(
		mustTestActivity(t, 1),
		now,
		now.Add(time.Hour),
		mustTestGraphReference(t, 1, "graph:r1"),
		testStrategyManifest(t),
		mustTestEvidence(t, "approval/release"),
		now,
	)
	if err != nil {
		t.Fatalf("prepare transition: %v", err)
	}
	tests := []struct {
		name   string
		value  ActivityTransition
		mutate func(*ActivityTransition)
	}{
		{name: "zero", value: ActivityTransition{}},
		{name: "expected lifecycle", value: valid, mutate: func(value *ActivityTransition) { value.expectedLifecycle = ActivityLifecycleRetired }},
		{name: "expected active", value: valid, mutate: func(value *ActivityTransition) { value.expectedActivePublicationVersion = 9 }},
		{name: "next generation", value: valid, mutate: func(value *ActivityTransition) { value.next.stateVersion = 2; value.next.activePublicationVersion = 2 }},
		{name: "invalid record", value: valid, mutate: func(value *ActivityTransition) { value.record.schemaVersion = 0 }},
		{name: "wrong record id", value: valid, mutate: func(value *ActivityTransition) { value.record.activityID = 2 }},
		{name: "wrong record version", value: valid, mutate: func(value *ActivityTransition) { value.record.version = 2 }},
		{name: "missing append flag", value: valid, mutate: func(value *ActivityTransition) { value.appendsPublication = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := test.value
			value.record = test.value.record.clone()
			if test.mutate != nil {
				test.mutate(&value)
			}
			if err := value.Validate(); !errors.Is(err, ErrActivityTransitionInvalid) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestActivityTransitionRecordIsDefensivelyCopied(t *testing.T) {
	now := testMarketingInstant()
	transition, err := PlanPublish(
		mustTestActivity(t, 1),
		now,
		now.Add(time.Hour),
		mustTestGraphReference(t, 1, "graph:r1"),
		testStrategyManifest(t),
		mustTestEvidence(t, "approval/release"),
		now,
	)
	if err != nil {
		t.Fatalf("PlanPublish: %v", err)
	}
	first, _ := transition.Record()
	first.strategyRevisionManifest[0] = mustTestStrategyReference(t, 99, "mutated:r1")
	second, _ := transition.Record()
	if second.strategyRevisionManifest[0].StrategyID() != 11 {
		t.Fatal("Record exposes transition-owned manifest storage")
	}
}

func assertPublishTransitionCAS(
	t *testing.T,
	transition ActivityTransition,
	expectedLifecycle ActivityLifecycle,
	expectedState ActivityStateVersion,
	expectedActive ActivityPublicationVersion,
	nextVersion ActivityPublicationVersion,
) {
	t.Helper()
	if err := transition.Validate(); err != nil {
		t.Fatalf("transition.Validate: %v", err)
	}
	if transition.ExpectedLifecycle() != expectedLifecycle ||
		transition.ExpectedStateVersion() != expectedState ||
		transition.ExpectedActivePublicationVersion() != expectedActive ||
		!transition.AppendsPublication() {
		t.Fatalf("transition CAS = %#v", transition)
	}
	next := transition.Next()
	if next.Lifecycle() != ActivityLifecyclePublished ||
		next.StateVersion() != ActivityStateVersion(nextVersion) ||
		next.ActivePublicationVersion() != nextVersion {
		t.Fatalf("next state = %q/%d/%d", next.Lifecycle(), next.StateVersion(), next.ActivePublicationVersion())
	}
}

func assertStrategyManifestsEqual(
	t *testing.T,
	left []LotteryStrategyRevisionReference,
	right []LotteryStrategyRevisionReference,
) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("manifest lengths = %d/%d", len(left), len(right))
	}
	for index := range left {
		if left[index] != right[index] {
			t.Fatalf("manifest[%d] = %#v/%#v", index, left[index], right[index])
		}
	}
}
