package lotteryconfig

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	lotteryapplication "github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	lotterydomain "github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	marketingapplication "github.com/Atingaii/GrowthOS-Go/internal/marketing/application"
	marketingdomain "github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

type graphReaderFunc func(
	context.Context,
	lotterydomain.StrategyRoutingGraphIdentity,
) (lotterydomain.StrategyRoutingGraph, error)

func (function graphReaderFunc) FindByIdentity(
	ctx context.Context,
	identity lotterydomain.StrategyRoutingGraphIdentity,
) (lotterydomain.StrategyRoutingGraph, error) {
	return function(ctx, identity)
}

type snapshotReaderFunc func(
	context.Context,
	lotterydomain.StrategySnapshotIdentity,
) (lotterydomain.StrategySnapshot, error)

func (function snapshotReaderFunc) FindSnapshotByIdentity(
	ctx context.Context,
	identity lotterydomain.StrategySnapshotIdentity,
) (lotterydomain.StrategySnapshot, error) {
	return function(ctx, identity)
}

func TestVerifierReadsExactGraphAndEveryExactStrategySnapshot(t *testing.T) {
	graph := testGraph(t, 11, 22)
	candidate := testCandidate(t, []strategyReference{
		{id: 11, revision: "strategy-r3"},
		{id: 22, revision: "strategy-r8"},
	})
	var graphIdentities []lotterydomain.StrategyRoutingGraphIdentity
	var strategyIdentities []lotterydomain.StrategySnapshotIdentity
	verifier, err := NewVerifier(
		graphReaderFunc(func(_ context.Context, identity lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
			graphIdentities = append(graphIdentities, identity)
			return graph, nil
		}),
		snapshotReaderFunc(func(_ context.Context, identity lotterydomain.StrategySnapshotIdentity) (lotterydomain.StrategySnapshot, error) {
			strategyIdentities = append(strategyIdentities, identity)
			return testSnapshot(t, identity.ID(), string(identity.Revision())), nil
		}),
	)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}

	if err := verifier.VerifyPublication(context.Background(), candidate); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(graphIdentities) != 1 || graphIdentities[0] != graph.Identity() {
		t.Fatalf("graph identities = %v", graphIdentities)
	}
	wantStrategyIDs := []lotterydomain.StrategyID{11, 22}
	gotStrategyIDs := make([]lotterydomain.StrategyID, 0, len(strategyIdentities))
	for _, identity := range strategyIdentities {
		gotStrategyIDs = append(gotStrategyIDs, identity.ID())
	}
	if !slices.Equal(gotStrategyIDs, wantStrategyIDs) {
		t.Fatalf("Strategy IDs = %v, want %v", gotStrategyIDs, wantStrategyIDs)
	}
}

func TestVerifierUsesUniqueTerminalStrategySet(t *testing.T) {
	graph := testGraph(t, 11, 11)
	candidate := testCandidate(t, []strategyReference{{id: 11, revision: "strategy-r3"}})
	reads := 0
	verifier, err := NewVerifier(
		graphReaderFunc(func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
			return graph, nil
		}),
		snapshotReaderFunc(func(_ context.Context, identity lotterydomain.StrategySnapshotIdentity) (lotterydomain.StrategySnapshot, error) {
			reads++
			return testSnapshot(t, identity.ID(), string(identity.Revision())), nil
		}),
	)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	if err := verifier.VerifyPublication(context.Background(), candidate); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if reads != 1 {
		t.Fatalf("snapshot reads = %d, want unique set read once", reads)
	}
}

func TestVerifierRejectsMissingAndExtraManifestBeforeSnapshotReads(t *testing.T) {
	for _, test := range []struct {
		name     string
		graph    lotterydomain.StrategyRoutingGraph
		manifest []strategyReference
	}{
		{
			name:     "missing terminal",
			graph:    testGraph(t, 11, 22),
			manifest: []strategyReference{{id: 11, revision: "strategy-r3"}},
		},
		{
			name:  "extra manifest entry",
			graph: testGraph(t, 11, 11),
			manifest: []strategyReference{
				{id: 11, revision: "strategy-r3"},
				{id: 22, revision: "strategy-r8"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			verifier, err := NewVerifier(
				graphReaderFunc(func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
					return test.graph, nil
				}),
				snapshotReaderFunc(func(context.Context, lotterydomain.StrategySnapshotIdentity) (lotterydomain.StrategySnapshot, error) {
					reads++
					return lotterydomain.StrategySnapshot{}, nil
				}),
			)
			if err != nil {
				t.Fatalf("new verifier: %v", err)
			}
			err = verifier.VerifyPublication(context.Background(), testCandidate(t, test.manifest))
			if !errors.Is(err, marketingapplication.ErrLotteryPublicationInvalid) {
				t.Fatalf("error = %v, want invalid", err)
			}
			if reads != 0 {
				t.Fatalf("snapshot reads = %d, want 0", reads)
			}
		})
	}
}

func TestVerifierRejectsWrongExactIdentitiesAndClassifiesReads(t *testing.T) {
	validGraph := testGraph(t, 11, 22)
	validCandidate := testCandidate(t, []strategyReference{
		{id: 11, revision: "strategy-r3"},
		{id: 22, revision: "strategy-r8"},
	})
	otherGraph := testGraphWithIdentity(t, 99, "other-r1", 11, 22)
	private := errors.New("private Lottery SQL and credentials")

	for _, test := range []struct {
		name         string
		graphRead    graphReaderFunc
		strategyRead snapshotReaderFunc
		want         error
	}{
		{
			name: "wrong graph identity",
			graphRead: func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
				return otherGraph, nil
			},
			strategyRead: validSnapshotReader(t),
			want:         marketingapplication.ErrLotteryPublicationInvalid,
		},
		{
			name: "graph missing",
			graphRead: func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
				return lotterydomain.StrategyRoutingGraph{}, lotteryapplication.ErrStrategyRoutingGraphNotFound
			},
			strategyRead: validSnapshotReader(t),
			want:         marketingapplication.ErrLotteryPublicationInvalid,
		},
		{
			name: "graph unavailable",
			graphRead: func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
				return validGraph, lotteryapplication.WrapRepositoryError(lotteryapplication.ErrRepositoryFailure, private)
			},
			strategyRead: validSnapshotReader(t),
			want:         marketingapplication.ErrLotteryPublicationUnavailable,
		},
		{
			name: "Strategy missing",
			graphRead: func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
				return validGraph, nil
			},
			strategyRead: func(context.Context, lotterydomain.StrategySnapshotIdentity) (lotterydomain.StrategySnapshot, error) {
				return lotterydomain.StrategySnapshot{}, lotteryapplication.ErrStrategySnapshotNotFound
			},
			want: marketingapplication.ErrLotteryPublicationInvalid,
		},
		{
			name: "wrong Strategy identity",
			graphRead: func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
				return validGraph, nil
			},
			strategyRead: func(context.Context, lotterydomain.StrategySnapshotIdentity) (lotterydomain.StrategySnapshot, error) {
				return testSnapshot(t, 99, "wrong-r1"), nil
			},
			want: marketingapplication.ErrLotteryPublicationInvalid,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier, err := NewVerifier(test.graphRead, test.strategyRead)
			if err != nil {
				t.Fatalf("new verifier: %v", err)
			}
			err = verifier.VerifyPublication(context.Background(), validCandidate)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if err.Error() != test.want.Error() || errors.Is(err, private) {
				t.Fatalf("error leaked cause: %q", err)
			}
		})
	}
}

func TestVerifierCancellationWinsOverDependencyError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	verifier, err := NewVerifier(
		graphReaderFunc(func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
			cancel()
			return lotterydomain.StrategyRoutingGraph{}, errors.New("dependency error")
		}),
		validSnapshotReader(t),
	)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	err = verifier.VerifyPublication(ctx, testCandidate(t, []strategyReference{
		{id: 11, revision: "strategy-r3"},
		{id: 22, revision: "strategy-r8"},
	}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want caller cancellation", err)
	}
}

func TestVerifierRejectsTypedNilAndZeroCandidate(t *testing.T) {
	var typedNilGraphReader graphReaderFunc
	if _, err := NewVerifier(typedNilGraphReader, validSnapshotReader(t)); !errors.Is(err, marketingapplication.ErrLotteryPublicationUnavailable) {
		t.Fatalf("typed-nil error = %v", err)
	}
	graphReads := 0
	verifier, err := NewVerifier(
		graphReaderFunc(func(context.Context, lotterydomain.StrategyRoutingGraphIdentity) (lotterydomain.StrategyRoutingGraph, error) {
			graphReads++
			return lotterydomain.StrategyRoutingGraph{}, nil
		}),
		validSnapshotReader(t),
	)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	err = verifier.VerifyPublication(context.Background(), marketingapplication.ActivityPublicationCandidate{})
	if !errors.Is(err, marketingapplication.ErrLotteryPublicationInvalid) || graphReads != 0 {
		t.Fatalf("error/reads = %v/%d", err, graphReads)
	}
}

type strategyReference struct {
	id       marketingdomain.LotteryStrategyID
	revision string
}

func testCandidate(
	t *testing.T,
	references []strategyReference,
) marketingapplication.ActivityPublicationCandidate {
	t.Helper()
	activity, err := marketingdomain.NewActivity(41, "Campaign")
	if err != nil {
		t.Fatalf("new Activity: %v", err)
	}
	graphReference, err := marketingdomain.NewLotteryGraphReference(7, "graph-r1")
	if err != nil {
		t.Fatalf("new graph reference: %v", err)
	}
	manifest := make([]marketingdomain.LotteryStrategyRevisionReference, 0, len(references))
	for _, reference := range references {
		value, err := marketingdomain.NewLotteryStrategyRevisionReference(reference.id, reference.revision)
		if err != nil {
			t.Fatalf("new Strategy reference: %v", err)
		}
		manifest = append(manifest, value)
	}
	publishedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	transition, err := marketingdomain.PlanPublish(
		activity,
		publishedAt.Add(-time.Hour),
		publishedAt.Add(time.Hour),
		graphReference,
		manifest,
		marketingdomain.EvidenceReference("governance/accepted"),
		publishedAt,
	)
	if err != nil {
		t.Fatalf("plan publication: %v", err)
	}
	publication, ok := transition.Record()
	if !ok {
		t.Fatal("publication plan has no record")
	}
	candidate, err := marketingapplication.NewActivityPublicationCandidate(publication)
	if err != nil {
		t.Fatalf("new candidate: %v", err)
	}
	return candidate
}

func testGraph(
	t *testing.T,
	premiumStrategy lotterydomain.StrategyID,
	baselineStrategy lotterydomain.StrategyID,
) lotterydomain.StrategyRoutingGraph {
	t.Helper()
	return testGraphWithIdentity(t, 7, "graph-r1", premiumStrategy, baselineStrategy)
}

func testGraphWithIdentity(
	t *testing.T,
	graphID lotterydomain.StrategyRoutingGraphID,
	revision string,
	premiumStrategy lotterydomain.StrategyID,
	baselineStrategy lotterydomain.StrategyID,
) lotterydomain.StrategyRoutingGraph {
	t.Helper()
	root, err := lotterydomain.NewStrategyRoutingDecisionNode(1, lotterydomain.MembershipStrategyRoutingRuleCode)
	if err != nil {
		t.Fatalf("new root: %v", err)
	}
	premium, err := lotterydomain.NewStrategyRoutingTargetNode(2, premiumStrategy)
	if err != nil {
		t.Fatalf("new premium target: %v", err)
	}
	baseline, err := lotterydomain.NewStrategyRoutingTargetNode(3, baselineStrategy)
	if err != nil {
		t.Fatalf("new baseline target: %v", err)
	}
	premiumEdge, err := lotterydomain.NewStrategyRoutingEdge(1, 2, lotterydomain.MembershipRoutingBranchPremiumOverride)
	if err != nil {
		t.Fatalf("new premium edge: %v", err)
	}
	baselineEdge, err := lotterydomain.NewStrategyRoutingEdge(1, 3, lotterydomain.MembershipRoutingBranchBaselineDefault)
	if err != nil {
		t.Fatalf("new baseline edge: %v", err)
	}
	graph, err := lotterydomain.NewStrategyRoutingGraph(
		graphID,
		revision,
		1,
		[]lotterydomain.StrategyRoutingNode{root, premium, baseline},
		[]lotterydomain.StrategyRoutingEdge{premiumEdge, baselineEdge},
	)
	if err != nil {
		t.Fatalf("new graph: %v", err)
	}
	return graph
}

func testSnapshot(
	t *testing.T,
	strategyID lotterydomain.StrategyID,
	revision string,
) lotterydomain.StrategySnapshot {
	t.Helper()
	award, err := lotterydomain.NewAward(1, "Reward", 1, lotterydomain.AwardOutcomeReward)
	if err != nil {
		t.Fatalf("new Award: %v", err)
	}
	strategy, err := lotterydomain.NewStrategy(strategyID, "Strategy", []lotterydomain.Award{award})
	if err != nil {
		t.Fatalf("new Strategy: %v", err)
	}
	snapshot, err := lotterydomain.NewStrategySnapshot(revision, strategy)
	if err != nil {
		t.Fatalf("new snapshot: %v", err)
	}
	return snapshot
}

func validSnapshotReader(t *testing.T) snapshotReaderFunc {
	t.Helper()
	return func(_ context.Context, identity lotterydomain.StrategySnapshotIdentity) (lotterydomain.StrategySnapshot, error) {
		return testSnapshot(t, identity.ID(), string(identity.Revision())), nil
	}
}
