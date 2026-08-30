package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestLotteryForeignReferencesRemainExactBoundedLocalValues(t *testing.T) {
	boundary := "r" + strings.Repeat("1", MaxForeignRevisionBytes-1)
	graph, err := NewLotteryGraphReference(7, boundary)
	if err != nil {
		t.Fatalf("NewLotteryGraphReference: %v", err)
	}
	if graph.ID() != 7 || graph.Revision() != LotteryRevision(boundary) {
		t.Fatalf("graph reference = %d/%q", graph.ID(), graph.Revision())
	}
	strategy, err := NewLotteryStrategyRevisionReference(8, "strategy:r9")
	if err != nil {
		t.Fatalf("NewLotteryStrategyRevisionReference: %v", err)
	}
	if strategy.StrategyID() != 8 || strategy.Revision() != "strategy:r9" {
		t.Fatalf("Strategy reference = %d/%q", strategy.StrategyID(), strategy.Revision())
	}
}

func TestLotteryForeignReferencesRejectInvalidIdentityAndRevision(t *testing.T) {
	invalidRevisions := []string{
		"",
		"-leading",
		"has/slash",
		"has space",
		"emoji🙂",
		strings.Repeat("r", MaxForeignRevisionBytes+1),
	}
	for _, revision := range invalidRevisions {
		graph, graphErr := NewLotteryGraphReference(1, revision)
		if !errors.Is(graphErr, ErrLotteryGraphReferenceInvalid) || graph != (LotteryGraphReference{}) {
			t.Fatalf("graph revision %q = %#v, %v", revision, graph, graphErr)
		}
		strategy, strategyErr := NewLotteryStrategyRevisionReference(1, revision)
		if !errors.Is(strategyErr, ErrLotteryStrategyRevisionReferenceInvalid) ||
			strategy != (LotteryStrategyRevisionReference{}) {
			t.Fatalf("Strategy revision %q = %#v, %v", revision, strategy, strategyErr)
		}
	}
	if _, err := NewLotteryGraphReference(0, "r1"); !errors.Is(err, ErrLotteryGraphReferenceInvalid) {
		t.Fatalf("zero graph id error = %v", err)
	}
	if _, err := NewLotteryStrategyRevisionReference(0, "r1"); !errors.Is(err, ErrLotteryStrategyRevisionReferenceInvalid) {
		t.Fatalf("zero Strategy id error = %v", err)
	}
}

func TestCanonicalStrategyRevisionManifestSortsAndRejectsAmbiguity(t *testing.T) {
	third := mustTestStrategyReference(t, 3, "r3")
	first := mustTestStrategyReference(t, 1, "r1")
	second := mustTestStrategyReference(t, 2, "r2")
	canonical, err := canonicalStrategyRevisionManifest(
		[]LotteryStrategyRevisionReference{third, first, second},
	)
	if err != nil {
		t.Fatalf("canonicalStrategyRevisionManifest: %v", err)
	}
	for index, id := range []LotteryStrategyID{1, 2, 3} {
		if canonical[index].StrategyID() != id {
			t.Fatalf("canonical[%d] id = %d, want %d", index, canonical[index].StrategyID(), id)
		}
	}

	tests := []struct {
		name     string
		manifest []LotteryStrategyRevisionReference
		want     error
	}{
		{name: "empty", want: ErrStrategyRevisionManifestInvalid},
		{name: "same duplicate", manifest: []LotteryStrategyRevisionReference{first, first}, want: ErrStrategyRevisionManifestInvalid},
		{name: "ambiguous revisions", manifest: []LotteryStrategyRevisionReference{first, {strategyID: 1, revision: "r2"}}, want: ErrStrategyRevisionManifestInvalid},
		{name: "zero entry", manifest: []LotteryStrategyRevisionReference{{}}, want: ErrStrategyRevisionManifestInvalid},
		{name: "over limit", manifest: strategyManifestWithCount(MaxStrategyRevisionManifestEntries + 1), want: ErrStrategyRevisionManifestLimitExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canonical, err := canonicalStrategyRevisionManifest(test.manifest)
			if !errors.Is(err, test.want) || canonical != nil {
				t.Fatalf("canonicalStrategyRevisionManifest() = %#v, %v, want %v", canonical, err, test.want)
			}
		})
	}
}

func strategyManifestWithCount(count int) []LotteryStrategyRevisionReference {
	manifest := make([]LotteryStrategyRevisionReference, count)
	for index := range manifest {
		manifest[index] = LotteryStrategyRevisionReference{
			strategyID: LotteryStrategyID(index + 1),
			revision:   "r1",
		}
	}
	return manifest
}
