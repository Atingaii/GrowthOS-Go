package domain

import "testing"

func FuzzRestoreStrategyRoutingGraphTopologyNeverPanicsOrLoops(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{255, 255, 255, 255})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) == 0 {
			nodes, edges := minimalStrategyRoutingGraphParts(t)
			graph, err := RestoreStrategyRoutingGraph(
				1,
				"fuzz-v1",
				StrategyRoutingGraphSchemaVersionV1,
				1,
				nodes,
				edges,
			)
			if err != nil || graph.Validate() != nil {
				t.Fatalf("valid seed failed: graph=%#v err=%v", graph, err)
			}
			return
		}
		if len(input) > 4096 {
			input = input[:4096]
		}
		at := func(index int) byte { return input[index%len(input)] }

		additionalNodeCount := int(at(0)) % (MaxStrategyRoutingGraphNodes + 2)
		nodes := make([]StrategyRoutingNode, 0, additionalNodeCount+2)
		nodes = append(nodes,
			StrategyRoutingNode{
				id:       1,
				kind:     StrategyRoutingNodeKindDecision,
				ruleCode: MembershipStrategyRoutingRuleCode,
			},
			StrategyRoutingNode{
				id:         2,
				kind:       StrategyRoutingNodeKindStrategyTarget,
				strategyID: 1,
			},
		)
		for index := 0; index < additionalNodeCount; index++ {
			id := StrategyRoutingNodeID(at(index*5+1)) + 1
			switch at(index*5+2) % 3 {
			case 0:
				nodes = append(nodes, StrategyRoutingNode{
					id:       id,
					kind:     StrategyRoutingNodeKindDecision,
					ruleCode: MembershipStrategyRoutingRuleCode,
				})
			case 1:
				nodes = append(nodes, StrategyRoutingNode{
					id:         id,
					kind:       StrategyRoutingNodeKindStrategyTarget,
					strategyID: StrategyID(at(index*5+3)) + 1,
				})
			default:
				nodes = append(nodes, StrategyRoutingNode{
					id:         id,
					kind:       StrategyRoutingNodeKind(string([]byte{at(index*5 + 3)})),
					ruleCode:   MembershipStrategyRoutingRuleCode,
					strategyID: StrategyID(at(index*5 + 4)),
				})
			}
		}

		additionalEdgeCount := int(at(1)) % (MaxStrategyRoutingGraphEdges + 2)
		edges := make([]StrategyRoutingEdge, 0, additionalEdgeCount+2)
		edges = append(edges,
			StrategyRoutingEdge{
				from:   1,
				to:     2,
				branch: MembershipRoutingBranchPremiumOverride,
			},
			StrategyRoutingEdge{
				from:      1,
				to:        2,
				branch:    MembershipRoutingBranchBaselineDefault,
				isDefault: true,
			},
		)
		for index := 0; index < additionalEdgeCount; index++ {
			branch := MembershipRoutingBranchPremiumOverride
			isDefault := false
			switch at(index*4+2) % 3 {
			case 1:
				branch = MembershipRoutingBranchBaselineDefault
				isDefault = true
			case 2:
				branch = MembershipRoutingBranch(string([]byte{at(index*4 + 3)}))
				isDefault = at(index*4+4)%2 == 0
			}
			edges = append(edges, StrategyRoutingEdge{
				from:      StrategyRoutingNodeID(at(index*4 + 4)),
				to:        StrategyRoutingNodeID(at(index*4 + 5)),
				branch:    branch,
				isDefault: isDefault,
			})
		}

		graph, err := RestoreStrategyRoutingGraph(
			1,
			"fuzz-v1",
			StrategyRoutingGraphSchemaVersionV1,
			1,
			nodes,
			edges,
		)
		if err != nil {
			assertZeroStrategyRoutingGraph(t, graph)
			return
		}
		if err := graph.Validate(); err != nil {
			t.Fatalf("successful restore produced invalid graph: %v", err)
		}
		if len(graph.Nodes()) > MaxStrategyRoutingGraphNodes ||
			len(graph.Edges()) > MaxStrategyRoutingGraphEdges ||
			graph.Depth() > MaxStrategyRoutingGraphDepth {
			t.Fatalf("successful restore escaped bounds: %#v", graph)
		}
	})
}
