package domain

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func FuzzEvaluateStrategyRoutingGraphNeverPanicsLoopsOrReturnsPartialDecision(f *testing.F) {
	f.Add(uint8(1), uint8(0), uint8(1), false, uint8(0))
	f.Add(uint8(1), uint8(1), uint8(1), false, uint8(0))
	f.Add(uint8(16), uint8(1), uint8(16), false, uint8(0))
	f.Add(uint8(16), uint8(1), uint8(15), false, uint8(0))
	f.Add(uint8(8), uint8(2), uint8(8), false, uint8(0))
	f.Add(uint8(4), uint8(0), uint8(4), true, uint8(0))
	f.Add(uint8(4), uint8(1), uint8(17), false, uint8(3))
	f.Add(uint8(2), uint8(1), uint8(2), false, uint8(6))
	f.Add(uint8(2), uint8(1), uint8(2), false, uint8(7))
	f.Add(uint8(2), uint8(1), uint8(2), false, uint8(8))

	f.Fuzz(func(
		t *testing.T,
		depthSeed uint8,
		tierSeed uint8,
		budgetSeed uint8,
		future bool,
		mutation uint8,
	) {
		depth := int(depthSeed%MaxStrategyRoutingGraphDepth) + 1
		graph, err := strategyRoutingEvaluationFuzzGraph(depth)
		if err != nil {
			t.Fatalf("construct bounded fuzz graph: %v", err)
		}
		switch mutation % 9 {
		case 1:
			graph.depth = 0
		case 2:
			graph.rootNodeID = 0
		case 3:
			graph.edges[0].to = 0
		case 4:
			graph.nodes[0].kind = "unknown"
		case 5:
			graph.schemaVersion = StrategyRoutingGraphSchemaVersion(2)
		case 6:
			graph.id = 0
		case 7:
			graph.revision = ""
		case 8:
			graph.revision = "-invalid-revision"
		}

		evaluatedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
		observedAt := evaluatedAt.Add(-time.Minute)
		if future {
			observedAt = evaluatedAt.Add(time.Nanosecond)
		}
		tierMode := tierSeed % 3
		tier := MembershipTierStandard
		if tierMode == 1 {
			tier = MembershipTierPremium
		}
		fact, err := NewMembershipTierFactSnapshot(
			1,
			tier,
			observedAt,
			"membership-authority",
			"fuzz-revision-v1",
		)
		if err != nil {
			t.Fatalf("construct fuzz fact: %v", err)
		}
		if tierMode == 2 {
			fact.tier = "unknown"
		}

		maxSteps := int(budgetSeed % (MaxStrategyRoutingGraphDepth + 2))
		budget := StrategyRoutingGraphStepBudget{maxSteps: uint8(maxSteps)}
		decision, evaluationErr := EvaluateStrategyRoutingGraph(
			context.Background(),
			graph,
			fact,
			evaluatedAt,
			budget,
		)
		if evaluationErr != nil {
			assertEvaluationZeroDecision(t, decision)
			return
		}

		if mutation%9 != 0 || maxSteps < 1 || maxSteps > MaxStrategyRoutingGraphDepth ||
			graph.Depth() > maxSteps || tierMode == 2 || future {
			t.Fatalf(
				"invalid inputs unexpectedly succeeded: depth=%d tier=%d budget=%d future=%v mutation=%d decision=%#v",
				depth,
				tierMode,
				maxSteps,
				future,
				mutation%9,
				decision,
			)
		}
		path := decision.Path()
		if !decision.Confirmed() || len(path) < 1 || len(path) > maxSteps ||
			len(path) > MaxStrategyRoutingGraphDepth {
			t.Fatalf("success escaped bounds: decision=%#v path=%#v", decision, path)
		}
		if tier == MembershipTierPremium {
			if decision.Target() != 202 || len(path) != depth {
				t.Fatalf("premium path diverged: depth=%d decision=%#v", depth, decision)
			}
		} else if decision.Target() != 101 || len(path) != 1 {
			t.Fatalf("standard path diverged: depth=%d decision=%#v", depth, decision)
		}
	})
}

func strategyRoutingEvaluationFuzzGraph(depth int) (StrategyRoutingGraph, error) {
	nodes := make([]StrategyRoutingNode, 0, depth+2)
	edges := make([]StrategyRoutingEdge, 0, depth*2)
	for step := 1; step <= depth; step++ {
		nodeID := StrategyRoutingNodeID(step)
		node, err := NewStrategyRoutingDecisionNode(nodeID, MembershipStrategyRoutingRuleCode)
		if err != nil {
			return StrategyRoutingGraph{}, err
		}
		nodes = append(nodes, node)
		baseline, err := NewStrategyRoutingEdge(
			nodeID,
			1001,
			MembershipRoutingBranchBaselineDefault,
		)
		if err != nil {
			return StrategyRoutingGraph{}, err
		}
		edges = append(edges, baseline)
		premiumSuccessor := StrategyRoutingNodeID(step + 1)
		if step == depth {
			premiumSuccessor = 1002
		}
		premium, err := NewStrategyRoutingEdge(
			nodeID,
			premiumSuccessor,
			MembershipRoutingBranchPremiumOverride,
		)
		if err != nil {
			return StrategyRoutingGraph{}, err
		}
		edges = append(edges, premium)
	}
	baselineTarget, err := NewStrategyRoutingTargetNode(1001, 101)
	if err != nil {
		return StrategyRoutingGraph{}, err
	}
	premiumTarget, err := NewStrategyRoutingTargetNode(1002, 202)
	if err != nil {
		return StrategyRoutingGraph{}, err
	}
	nodes = append(nodes, baselineTarget, premiumTarget)
	return NewStrategyRoutingGraph(
		99,
		fmt.Sprintf("fuzz-depth-%d-v1", depth),
		1,
		nodes,
		edges,
	)
}
