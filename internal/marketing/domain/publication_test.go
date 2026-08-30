package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRestoreActivityPublicationBuildsExactImmutableRelease(t *testing.T) {
	start := testMarketingInstant().Add(time.Hour)
	end := start.Add(24 * time.Hour)
	publishedAt := start.Add(-time.Hour)
	manifest := testStrategyManifest(t)
	publication, err := RestoreActivityPublication(
		41,
		7,
		ActivityPublicationSchemaVersionV1,
		ActivityPublicationKindRelease,
		0,
		start,
		end,
		publishedAt,
		mustTestGraphReference(t, 9, "graph:r3"),
		manifest,
		mustTestEvidence(t, "approval/change-17"),
	)
	if err != nil {
		t.Fatalf("RestoreActivityPublication: %v", err)
	}
	if publication.ActivityID() != 41 || publication.Version() != 7 ||
		publication.SchemaVersion() != ActivityPublicationSchemaVersionV1 ||
		publication.Kind() != ActivityPublicationKindRelease {
		t.Fatalf(
			"identity/schema/kind = %d/%d/%d/%q",
			publication.ActivityID(),
			publication.Version(),
			publication.SchemaVersion(),
			publication.Kind(),
		)
	}
	if rollbackOf, ok := publication.RollbackOf(); ok || rollbackOf != 0 {
		t.Fatalf("release rollback-of = %d/%v", rollbackOf, ok)
	}
	if publication.StartsAt() != start || publication.EndsAt() != end ||
		publication.PublishedAt() != publishedAt {
		t.Fatalf(
			"times = %v/%v/%v",
			publication.StartsAt(),
			publication.EndsAt(),
			publication.PublishedAt(),
		)
	}
	if publication.GraphReference().ID() != 9 ||
		publication.GraphReference().Revision() != "graph:r3" ||
		publication.ApprovalEvidenceReference() != "approval/change-17" {
		t.Fatalf(
			"refs = %#v/%q",
			publication.GraphReference(),
			publication.ApprovalEvidenceReference(),
		)
	}
	canonical := publication.StrategyRevisionManifest()
	if len(canonical) != 2 || canonical[0].StrategyID() != 11 || canonical[1].StrategyID() != 22 {
		t.Fatalf("manifest is not canonical: %#v", canonical)
	}
	if exact, found := publication.StrategyRevision(22); !found || exact.Revision() != "strategy-22:r4" {
		t.Fatalf("StrategyRevision(22) = %#v/%v", exact, found)
	}
	if exact, found := publication.StrategyRevision(999); found || exact != (LotteryStrategyRevisionReference{}) {
		t.Fatalf("StrategyRevision(999) = %#v/%v", exact, found)
	}
	if err := publication.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	manifest[0] = mustTestStrategyReference(t, 99, "mutated:r1")
	if publication.StrategyRevisionManifest()[1].StrategyID() != 22 {
		t.Fatal("publication aliases constructor manifest")
	}
	returned := publication.StrategyRevisionManifest()
	returned[0] = mustTestStrategyReference(t, 88, "mutated:r2")
	if publication.StrategyRevisionManifest()[0].StrategyID() != 11 {
		t.Fatal("publication aliases returned manifest")
	}
}

func TestRestoreActivityPublicationAcceptsValidRollbackShape(t *testing.T) {
	start := testMarketingInstant()
	publication, err := RestoreActivityPublication(
		1,
		5,
		ActivityPublicationSchemaVersionV1,
		ActivityPublicationKindRollback,
		2,
		start,
		start.Add(time.Hour),
		start.Add(30*time.Minute),
		mustTestGraphReference(t, 4, "graph-v2"),
		[]LotteryStrategyRevisionReference{mustTestStrategyReference(t, 7, "strategy-v3")},
		mustTestEvidence(t, "approval/rollback-5"),
	)
	if err != nil {
		t.Fatalf("RestoreActivityPublication: %v", err)
	}
	if rollbackOf, ok := publication.RollbackOf(); !ok || rollbackOf != 2 {
		t.Fatalf("rollback-of = %d/%v", rollbackOf, ok)
	}
}

func TestActivityPublicationRejectsInvalidOrMixedImmutableState(t *testing.T) {
	start := testMarketingInstant()
	valid := mustTestReleasePublication(t, 1, 3, start, start.Add(time.Hour), start)
	tests := []struct {
		name   string
		mutate func(*ActivityPublication)
		want   error
	}{
		{name: "activity id", mutate: func(value *ActivityPublication) { value.activityID = 0 }, want: ErrActivityIDInvalid},
		{name: "version", mutate: func(value *ActivityPublication) { value.version = 0 }, want: ErrActivityPublicationInvalid},
		{name: "schema zero", mutate: func(value *ActivityPublication) { value.schemaVersion = 0 }, want: ErrActivityPublicationSchemaUnsupported},
		{name: "schema future", mutate: func(value *ActivityPublication) { value.schemaVersion = 2 }, want: ErrActivityPublicationSchemaUnsupported},
		{name: "kind zero", mutate: func(value *ActivityPublication) { value.kind = "" }, want: ErrActivityPublicationKindUnsupported},
		{name: "kind future", mutate: func(value *ActivityPublication) { value.kind = "replace" }, want: ErrActivityPublicationKindUnsupported},
		{name: "release rollback source", mutate: func(value *ActivityPublication) { value.rollbackOf = 1 }, want: ErrActivityPublicationInvalid},
		{name: "rollback missing source", mutate: func(value *ActivityPublication) { value.kind = ActivityPublicationKindRollback }, want: ErrActivityPublicationInvalid},
		{name: "rollback same source", mutate: func(value *ActivityPublication) {
			value.kind = ActivityPublicationKindRollback
			value.rollbackOf = value.version
		}, want: ErrActivityPublicationInvalid},
		{name: "rollback future source", mutate: func(value *ActivityPublication) {
			value.kind = ActivityPublicationKindRollback
			value.rollbackOf = value.version + 1
		}, want: ErrActivityPublicationInvalid},
		{name: "start zero", mutate: func(value *ActivityPublication) { value.startsAt = time.Time{} }, want: ErrActivityPublicationWindowInvalid},
		{name: "end zero", mutate: func(value *ActivityPublication) { value.endsAt = time.Time{} }, want: ErrActivityPublicationWindowInvalid},
		{name: "published zero", mutate: func(value *ActivityPublication) { value.publishedAt = time.Time{} }, want: ErrActivityPublicationWindowInvalid},
		{name: "start nanos", mutate: func(value *ActivityPublication) { value.startsAt = value.startsAt.Add(time.Nanosecond) }, want: ErrActivityPublicationWindowInvalid},
		{name: "end non UTC", mutate: func(value *ActivityPublication) { value.endsAt = value.endsAt.In(time.FixedZone("CST", 8*60*60)) }, want: ErrActivityPublicationWindowInvalid},
		{name: "published nanos", mutate: func(value *ActivityPublication) { value.publishedAt = value.publishedAt.Add(time.Nanosecond) }, want: ErrActivityPublicationWindowInvalid},
		{name: "empty window", mutate: func(value *ActivityPublication) { value.endsAt = value.startsAt }, want: ErrActivityPublicationWindowInvalid},
		{name: "reverse window", mutate: func(value *ActivityPublication) { value.endsAt = value.startsAt.Add(-time.Microsecond) }, want: ErrActivityPublicationWindowInvalid},
		{name: "published at end", mutate: func(value *ActivityPublication) { value.publishedAt = value.endsAt }, want: ErrActivityPublicationExpired},
		{name: "published after end", mutate: func(value *ActivityPublication) { value.publishedAt = value.endsAt.Add(time.Microsecond) }, want: ErrActivityPublicationExpired},
		{name: "graph", mutate: func(value *ActivityPublication) { value.graphReference = LotteryGraphReference{} }, want: ErrLotteryGraphReferenceInvalid},
		{name: "manifest empty", mutate: func(value *ActivityPublication) { value.strategyRevisionManifest = nil }, want: ErrStrategyRevisionManifestInvalid},
		{name: "manifest duplicate", mutate: func(value *ActivityPublication) {
			value.strategyRevisionManifest = []LotteryStrategyRevisionReference{value.strategyRevisionManifest[0], value.strategyRevisionManifest[0]}
		}, want: ErrStrategyRevisionManifestInvalid},
		{name: "manifest noncanonical", mutate: func(value *ActivityPublication) {
			value.strategyRevisionManifest[0], value.strategyRevisionManifest[1] = value.strategyRevisionManifest[1], value.strategyRevisionManifest[0]
		}, want: ErrStrategyRevisionManifestInvalid},
		{name: "manifest over limit", mutate: func(value *ActivityPublication) {
			value.strategyRevisionManifest = strategyManifestWithCount(MaxStrategyRevisionManifestEntries + 1)
		}, want: ErrStrategyRevisionManifestInvalid},
		{name: "approval", mutate: func(value *ActivityPublication) { value.approvalEvidenceReference = "bad ref" }, want: ErrEvidenceReferenceInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := valid.clone()
			test.mutate(&value)
			if err := value.Validate(); !errors.Is(err, test.want) || !errors.Is(err, ErrActivityPublicationInvalid) {
				t.Fatalf("Validate() error = %v, want %v and ErrActivityPublicationInvalid", err, test.want)
			}
		})
	}
}

func TestRestoreActivityPublicationRejectsNonCanonicalPersistedInstantsAndManifestLimits(t *testing.T) {
	start := testMarketingInstant()
	graph := mustTestGraphReference(t, 1, "graph:r1")
	evidence := mustTestEvidence(t, "approval/1")
	tests := []struct {
		name      string
		startsAt  time.Time
		endsAt    time.Time
		published time.Time
		manifest  []LotteryStrategyRevisionReference
		want      error
	}{
		{name: "start local", startsAt: start.In(time.FixedZone("UTC+8", 8*60*60)), endsAt: start.Add(time.Hour), published: start, manifest: testStrategyManifest(t), want: ErrActivityPublicationWindowInvalid},
		{name: "published nanosecond", startsAt: start, endsAt: start.Add(time.Hour), published: start.Add(time.Nanosecond), manifest: testStrategyManifest(t), want: ErrActivityPublicationWindowInvalid},
		{name: "too many strategies", startsAt: start, endsAt: start.Add(time.Hour), published: start, manifest: strategyManifestWithCount(MaxStrategyRevisionManifestEntries + 1), want: ErrStrategyRevisionManifestLimitExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publication, err := RestoreActivityPublication(
				1,
				1,
				ActivityPublicationSchemaVersionV1,
				ActivityPublicationKindRelease,
				0,
				test.startsAt,
				test.endsAt,
				test.published,
				graph,
				test.manifest,
				evidence,
			)
			if !errors.Is(err, test.want) || !publication.isZero() {
				t.Fatalf("RestoreActivityPublication() = %#v, %v, want %v", publication, err, test.want)
			}
		})
	}
}

func TestPublicationRevisionAndEvidenceBoundariesMatchPersistenceGrammar(t *testing.T) {
	tooLongRevision := "r" + strings.Repeat("x", MaxForeignRevisionBytes)
	if _, err := NewLotteryGraphReference(1, tooLongRevision); !errors.Is(err, ErrLotteryGraphReferenceInvalid) {
		t.Fatalf("long graph revision error = %v", err)
	}
	tooLongEvidence := EvidenceReference("a" + strings.Repeat("x", MaxEvidenceReferenceBytes))
	if err := tooLongEvidence.Validate(); !errors.Is(err, ErrEvidenceReferenceInvalid) {
		t.Fatalf("long evidence error = %v", err)
	}
}
