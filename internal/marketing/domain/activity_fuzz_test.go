package domain

import (
	"testing"
	"time"
)

func FuzzDecideActivityGatePreservesHalfOpenMicrosecondBoundary(f *testing.F) {
	for _, offset := range []int64{-1_001, -1_000, -999, -1, 0, 1, 999, 1_000, 9_999, 10_000, 10_001} {
		f.Add(offset)
	}
	start := testMarketingInstant()
	end := start.Add(10 * time.Microsecond)
	graph, err := NewLotteryGraphReference(1, "graph:r1")
	if err != nil {
		f.Fatalf("NewLotteryGraphReference: %v", err)
	}
	strategy, err := NewLotteryStrategyRevisionReference(1, "strategy:r1")
	if err != nil {
		f.Fatalf("NewLotteryStrategyRevisionReference: %v", err)
	}
	evidence, err := NewEvidenceReference("approval/fuzz")
	if err != nil {
		f.Fatalf("NewEvidenceReference: %v", err)
	}
	publication, err := RestoreActivityPublication(
		1,
		1,
		ActivityPublicationSchemaVersionV1,
		ActivityPublicationKindRelease,
		0,
		start,
		end,
		start.Add(-time.Hour),
		graph,
		[]LotteryStrategyRevisionReference{strategy},
		evidence,
	)
	if err != nil {
		f.Fatalf("RestoreActivityPublication: %v", err)
	}
	activity, err := RestoreActivity(1, "fuzz", ActivityLifecyclePublished, 1, 1, time.Time{}, "")
	if err != nil {
		f.Fatalf("RestoreActivity: %v", err)
	}

	f.Fuzz(func(t *testing.T, rawOffsetNanos int64) {
		offset := rawOffsetNanos % int64(40*time.Microsecond)
		at := start.Add(time.Duration(offset))
		decision, err := DecideActivityGate(activity, publication, at)
		if err != nil {
			t.Fatalf("DecideActivityGate(%d): %v", offset, err)
		}
		canonicalAt := canonicalUTCInstant(at)
		want := ActivityGateStatusActive
		switch {
		case canonicalAt.Before(start):
			want = ActivityGateStatusScheduled
		case !canonicalAt.Before(end):
			want = ActivityGateStatusEnded
		}
		if decision.Status() != want {
			t.Fatalf("offset %d canonical %v: status = %q, want %q", offset, canonicalAt, decision.Status(), want)
		}
		if decision.AllowsParticipation() != (want == ActivityGateStatusActive) {
			t.Fatalf("offset %d: allow = %v for %q", offset, decision.AllowsParticipation(), want)
		}
		if err := decision.Validate(); err != nil {
			t.Fatalf("formed decision invalid: %v", err)
		}
	})
}

func FuzzPlanPublishNeverAcceptsAmbiguousOrNonCanonicalManifest(f *testing.F) {
	f.Add(uint64(1), "r1", uint64(2), "r2")
	f.Add(uint64(2), "r2", uint64(1), "r1")
	f.Add(uint64(1), "r1", uint64(1), "r2")
	f.Add(uint64(0), "", uint64(2), "bad revision")
	now := testMarketingInstant()

	f.Fuzz(func(t *testing.T, firstID uint64, firstRevision string, secondID uint64, secondRevision string) {
		manifest := []LotteryStrategyRevisionReference{
			{strategyID: LotteryStrategyID(firstID), revision: LotteryRevision(firstRevision)},
			{strategyID: LotteryStrategyID(secondID), revision: LotteryRevision(secondRevision)},
		}
		transition, err := PlanPublish(
			mustTestActivity(t, 1),
			now,
			now.Add(time.Hour),
			mustTestGraphReference(t, 1, "graph:r1"),
			manifest,
			mustTestEvidence(t, "approval/fuzz"),
			now,
		)
		if err != nil {
			assertZeroTransition(t, transition)
			return
		}
		if firstID == 0 || secondID == 0 || firstID == secondID {
			t.Fatalf("ambiguous ids %d/%d formed a publication", firstID, secondID)
		}
		if err := transition.Validate(); err != nil {
			t.Fatalf("transition.Validate: %v", err)
		}
		record, ok := transition.Record()
		if !ok {
			t.Fatal("successful publish has no record")
		}
		canonical := record.StrategyRevisionManifest()
		if len(canonical) != 2 || canonical[0].StrategyID() >= canonical[1].StrategyID() {
			t.Fatalf("manifest is not unique canonical: %#v", canonical)
		}
		if err := canonical[0].Validate(); err != nil {
			t.Fatalf("first reference invalid: %v", err)
		}
		if err := canonical[1].Validate(); err != nil {
			t.Fatalf("second reference invalid: %v", err)
		}

		manifest[0] = LotteryStrategyRevisionReference{}
		canonical[0] = LotteryStrategyRevisionReference{}
		again, _ := transition.Record()
		if err := again.Validate(); err != nil {
			t.Fatalf("manifest mutation escaped defensive copies: %v", err)
		}
	})
}

func FuzzPlanRollbackNeverReusesTargetOrWrapsVersion(f *testing.F) {
	f.Add(uint64(2), uint64(1), true, int64(0))
	f.Add(uint64(2), uint64(2), true, int64(0))
	f.Add(uint64(2), uint64(1), false, int64(0))
	f.Add(^uint64(0), uint64(1), true, int64(0))
	f.Add(uint64(3), uint64(1), true, int64(time.Hour))
	base := testMarketingInstant()

	f.Fuzz(func(
		t *testing.T,
		rawActive uint64,
		rawTarget uint64,
		wasPublished bool,
		rawOffsetNanos int64,
	) {
		active := ActivityPublicationVersion(rawActive)
		current, currentErr := RestoreActivity(
			1,
			"fuzz",
			ActivityLifecyclePublished,
			ActivityStateVersion(active),
			active,
			time.Time{},
			"",
		)
		if currentErr != nil {
			return
		}
		target, targetErr := RestoreActivityPublication(
			1,
			ActivityPublicationVersion(rawTarget),
			ActivityPublicationSchemaVersionV1,
			ActivityPublicationKindRelease,
			0,
			base,
			base.Add(time.Hour),
			base,
			mustTestGraphReference(t, 1, "graph:r1"),
			[]LotteryStrategyRevisionReference{mustTestStrategyReference(t, 1, "strategy:r1")},
			mustTestEvidence(t, "approval/target"),
		)
		if targetErr != nil {
			return
		}
		offset := rawOffsetNanos % int64(2*time.Hour)
		at := base.Add(time.Duration(offset))
		transition, err := PlanRollback(
			current,
			target,
			wasPublished,
			mustTestEvidence(t, "approval/rollback"),
			at,
		)
		if err != nil {
			assertZeroTransition(t, transition)
			return
		}
		if active == maxActivityVersion || rawTarget == 0 || rawTarget >= rawActive ||
			!wasPublished || !canonicalUTCInstant(at).Before(target.EndsAt()) {
			t.Fatalf(
				"invalid rollback succeeded: active=%d target=%d published=%v at=%v",
				active,
				rawTarget,
				wasPublished,
				at,
			)
		}
		record, ok := transition.Record()
		if !ok || record.Version() != active+1 || record.Version() == target.Version() {
			t.Fatalf("rollback record version = %d/%v, target %d", record.Version(), ok, target.Version())
		}
		rollbackOf, isRollback := record.RollbackOf()
		if !isRollback || rollbackOf != target.Version() || record.StartsAt() != target.StartsAt() ||
			record.EndsAt() != target.EndsAt() || record.GraphReference() != target.GraphReference() {
			t.Fatalf("rollback failed exact-copy contract: %#v", record)
		}
		if err := transition.Validate(); err != nil {
			t.Fatalf("transition.Validate: %v", err)
		}
	})
}
