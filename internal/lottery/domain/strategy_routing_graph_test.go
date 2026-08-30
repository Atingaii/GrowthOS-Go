package domain

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestNewStrategyRoutingGraphCanonicalizesConcreteDAG(t *testing.T) {
	decision := mustRoutingDecisionNode(t, 10)
	premiumTarget := mustRoutingTargetNode(t, 30, 300)
	baselineTarget := mustRoutingTargetNode(t, 20, 200)
	premiumEdge := mustRoutingEdge(t, 10, 30, MembershipRoutingBranchPremiumOverride)
	baselineEdge := mustRoutingEdge(t, 10, 20, MembershipRoutingBranchBaselineDefault)

	graph, err := NewStrategyRoutingGraph(
		7,
		"membership.route:v1",
		10,
		[]StrategyRoutingNode{premiumTarget, decision, baselineTarget},
		[]StrategyRoutingEdge{premiumEdge, baselineEdge},
	)
	if err != nil {
		t.Fatalf("construct graph: %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("validate graph: %v", err)
	}
	if graph.ID() != 7 || graph.Revision() != "membership.route:v1" ||
		graph.SchemaVersion() != StrategyRoutingGraphSchemaVersionV1 ||
		graph.RootNodeID() != 10 || graph.Depth() != 1 {
		t.Fatalf(
			"unexpected header: id=%d revision=%q schema=%d root=%d depth=%d",
			graph.ID(),
			graph.Revision(),
			graph.SchemaVersion(),
			graph.RootNodeID(),
			graph.Depth(),
		)
	}
	nodes := graph.Nodes()
	if got := []StrategyRoutingNodeID{nodes[0].ID(), nodes[1].ID(), nodes[2].ID()}; !slices.Equal(got, []StrategyRoutingNodeID{10, 20, 30}) {
		t.Fatalf("nodes are not canonical: %v", got)
	}
	edges := graph.Edges()
	if len(edges) != 2 ||
		edges[0].Branch() != MembershipRoutingBranchBaselineDefault ||
		edges[1].Branch() != MembershipRoutingBranchPremiumOverride {
		t.Fatalf("edges are not canonical: %#v", edges)
	}
	root, found := graph.Node(10)
	if !found || root.Kind() != StrategyRoutingNodeKindDecision ||
		root.RuleCode() != MembershipStrategyRoutingRuleCode || root.StrategyID() != 0 {
		t.Fatalf("unexpected root: %#v, found=%v", root, found)
	}
	terminal, found := graph.Node(20)
	if !found || terminal.Kind() != StrategyRoutingNodeKindStrategyTarget ||
		terminal.RuleCode() != "" || terminal.StrategyID() != 200 {
		t.Fatalf("unexpected terminal: %#v, found=%v", terminal, found)
	}
	if _, found := graph.Node(999); found {
		t.Fatal("unknown node must not be found")
	}
	if outgoing := graph.OutgoingEdges(10); len(outgoing) != 2 ||
		outgoing[0].From() != 10 || outgoing[0].To() != 20 || !outgoing[0].IsDefault() ||
		outgoing[1].From() != 10 || outgoing[1].To() != 30 || outgoing[1].IsDefault() {
		t.Fatalf("unexpected outgoing edges: %#v", outgoing)
	}
	if outgoing := graph.OutgoingEdges(20); len(outgoing) != 0 {
		t.Fatalf("terminal has outgoing edges: %#v", outgoing)
	}
	if outgoing := graph.OutgoingEdges(999); len(outgoing) != 0 {
		t.Fatalf("unknown node has outgoing edges: %#v", outgoing)
	}
}

func TestStrategyRoutingGraphAllowsBothBranchesToConvergeOnOneTerminal(t *testing.T) {
	nodes := []StrategyRoutingNode{
		mustRoutingTargetNode(t, 2, 200),
		mustRoutingDecisionNode(t, 1),
	}
	edges := []StrategyRoutingEdge{
		mustRoutingEdge(t, 1, 2, MembershipRoutingBranchPremiumOverride),
		mustRoutingEdge(t, 1, 2, MembershipRoutingBranchBaselineDefault),
	}

	graph, err := NewStrategyRoutingGraph(8, "converged-v1", 1, nodes, edges)
	if err != nil {
		t.Fatalf("construct converged graph: %v", err)
	}
	if graph.Depth() != 1 || len(graph.Nodes()) != 2 || len(graph.Edges()) != 2 {
		t.Fatalf(
			"unexpected converged graph: depth=%d nodes=%d edges=%d",
			graph.Depth(),
			len(graph.Nodes()),
			len(graph.Edges()),
		)
	}
	outgoing := graph.OutgoingEdges(1)
	if len(outgoing) != 2 || outgoing[0].To() != 2 || outgoing[1].To() != 2 {
		t.Fatalf("branches did not preserve their shared terminal: %#v", outgoing)
	}
}

func TestStrategyRoutingGraphV1SchemaContractsAreLiteralAndBounded(t *testing.T) {
	if StrategyRoutingGraphSchemaVersionV1 != 1 {
		t.Fatalf("schema version changed: %d", StrategyRoutingGraphSchemaVersionV1)
	}
	if StrategyRoutingNodeKindDecision != "decision" ||
		StrategyRoutingNodeKindStrategyTarget != "strategy_target" {
		t.Fatalf(
			"node kinds changed: %q/%q",
			StrategyRoutingNodeKindDecision,
			StrategyRoutingNodeKindStrategyTarget,
		)
	}
	if MaxStrategyRoutingGraphRevisionBytes != 128 ||
		MaxStrategyRoutingGraphNodes != 128 ||
		MaxStrategyRoutingGraphEdges != 256 ||
		MaxStrategyRoutingGraphDepth != 16 {
		t.Fatalf(
			"bounds changed: revision=%d nodes=%d edges=%d depth=%d",
			MaxStrategyRoutingGraphRevisionBytes,
			MaxStrategyRoutingGraphNodes,
			MaxStrategyRoutingGraphEdges,
			MaxStrategyRoutingGraphDepth,
		)
	}
}

func TestStrategyRoutingNodeConstructorsRejectUnknownOrMixedVariants(t *testing.T) {
	tests := []struct {
		name      string
		construct func() (StrategyRoutingNode, error)
	}{
		{
			name: "zero decision id",
			construct: func() (StrategyRoutingNode, error) {
				return NewStrategyRoutingDecisionNode(0, MembershipStrategyRoutingRuleCode)
			},
		},
		{
			name: "unknown decision rule",
			construct: func() (StrategyRoutingNode, error) {
				return NewStrategyRoutingDecisionNode(1, "lottery.unknown.rule")
			},
		},
		{
			name: "zero target id",
			construct: func() (StrategyRoutingNode, error) {
				return NewStrategyRoutingTargetNode(0, 1)
			},
		},
		{
			name: "zero Strategy target",
			construct: func() (StrategyRoutingNode, error) {
				return NewStrategyRoutingTargetNode(1, 0)
			},
		},
		{
			name: "unknown restored kind",
			construct: func() (StrategyRoutingNode, error) {
				return RestoreStrategyRoutingNode(1, "condition", MembershipStrategyRoutingRuleCode, 0)
			},
		},
		{
			name: "decision carries target",
			construct: func() (StrategyRoutingNode, error) {
				return RestoreStrategyRoutingNode(1, StrategyRoutingNodeKindDecision, MembershipStrategyRoutingRuleCode, 9)
			},
		},
		{
			name: "terminal carries rule",
			construct: func() (StrategyRoutingNode, error) {
				return RestoreStrategyRoutingNode(1, StrategyRoutingNodeKindStrategyTarget, MembershipStrategyRoutingRuleCode, 9)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node, err := test.construct()
			if !errors.Is(err, ErrStrategyRoutingNodeInvalid) {
				t.Fatalf("error = %v, want invalid node", err)
			}
			if node != (StrategyRoutingNode{}) {
				t.Fatalf("failure returned node: %#v", node)
			}
		})
	}
	if !errors.Is((StrategyRoutingNode{}).Validate(), ErrStrategyRoutingNodeInvalid) {
		t.Fatal("zero node must fail closed")
	}
}

func TestStrategyRoutingEdgeConstructionAndRestoreDefaultSemantics(t *testing.T) {
	tests := []struct {
		name        string
		branch      MembershipRoutingBranch
		wantDefault bool
	}{
		{name: "premium override", branch: MembershipRoutingBranchPremiumOverride},
		{name: "baseline default", branch: MembershipRoutingBranchBaselineDefault, wantDefault: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			edge, err := NewStrategyRoutingEdge(1, 2, test.branch)
			if err != nil {
				t.Fatalf("construct edge: %v", err)
			}
			if edge.From() != 1 || edge.To() != 2 || edge.Branch() != test.branch ||
				edge.IsDefault() != test.wantDefault {
				t.Fatalf("unexpected edge: %#v", edge)
			}
		})
	}

	invalid := []struct {
		name      string
		from      StrategyRoutingNodeID
		to        StrategyRoutingNodeID
		branch    MembershipRoutingBranch
		isDefault bool
	}{
		{name: "zero source", to: 2, branch: MembershipRoutingBranchPremiumOverride},
		{name: "zero destination", from: 1, branch: MembershipRoutingBranchPremiumOverride},
		{name: "unknown branch", from: 1, to: 2, branch: "standard"},
		{name: "premium marked default", from: 1, to: 2, branch: MembershipRoutingBranchPremiumOverride, isDefault: true},
		{name: "baseline not marked default", from: 1, to: 2, branch: MembershipRoutingBranchBaselineDefault},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			edge, err := RestoreStrategyRoutingEdge(
				test.from,
				test.to,
				test.branch,
				test.isDefault,
			)
			if !errors.Is(err, ErrStrategyRoutingEdgeInvalid) {
				t.Fatalf("error = %v, want invalid edge", err)
			}
			if edge != (StrategyRoutingEdge{}) {
				t.Fatalf("failure returned edge: %#v", edge)
			}
		})
	}
	if !errors.Is((StrategyRoutingEdge{}).Validate(), ErrStrategyRoutingEdgeInvalid) {
		t.Fatal("zero edge must fail closed")
	}
}

func TestStrategyRoutingGraphRejectsInvalidIdentityRevisionAndSchema(t *testing.T) {
	nodes, edges := minimalStrategyRoutingGraphParts(t)
	tests := []struct {
		name     string
		id       StrategyRoutingGraphID
		revision StrategyRoutingGraphRevision
		schema   StrategyRoutingGraphSchemaVersion
		want     error
	}{
		{name: "zero graph id", revision: "route-v1", schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "empty revision", id: 1, schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "leading punctuation", id: 1, revision: "-route-v1", schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "leading whitespace", id: 1, revision: " route-v1", schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "embedded whitespace", id: 1, revision: "route v1", schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "slash", id: 1, revision: "route/v1", schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "non ASCII", id: 1, revision: "route-版本", schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "too long", id: 1, revision: StrategyRoutingGraphRevision("r" + strings.Repeat("1", MaxStrategyRoutingGraphRevisionBytes)), schema: 1, want: ErrStrategyRoutingGraphIdentityInvalid},
		{name: "zero schema", id: 1, revision: "route-v1", want: ErrStrategyRoutingGraphSchemaUnsupported},
		{name: "future schema", id: 1, revision: "route-v1", schema: 2, want: ErrStrategyRoutingGraphSchemaUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, err := RestoreStrategyRoutingGraph(
				test.id,
				test.revision,
				test.schema,
				1,
				nodes,
				edges,
			)
			if !errors.Is(err, ErrStrategyRoutingGraphInvalid) || !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want graph invalid and %v", err, test.want)
			}
			assertZeroStrategyRoutingGraph(t, graph)
		})
	}

	boundary := "r" + strings.Repeat("1", MaxStrategyRoutingGraphRevisionBytes-1)
	graph, err := NewStrategyRoutingGraph(1, boundary, 1, nodes, edges)
	if err != nil {
		t.Fatalf("128-byte canonical revision should be valid: %v", err)
	}
	if graph.Revision() != StrategyRoutingGraphRevision(boundary) {
		t.Fatalf("revision changed: %q", graph.Revision())
	}
	if graph.SchemaVersion() != StrategyRoutingGraphSchemaVersionV1 {
		t.Fatalf("New must fix schema v1, got %d", graph.SchemaVersion())
	}
	identity := graph.Identity()
	if identity.ID() != graph.ID() || identity.Revision() != graph.Revision() {
		t.Fatalf("identity differs from graph header: %#v %#v", identity, graph)
	}
}

func TestStrategyRoutingGraphIdentityIsTheSharedFailClosedLookupValue(t *testing.T) {
	identity, err := NewStrategyRoutingGraphIdentity(19, "route.release:2026-08-30")
	if err != nil {
		t.Fatalf("construct identity: %v", err)
	}
	if identity.ID() != 19 || identity.Revision() != "route.release:2026-08-30" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("validate identity: %v", err)
	}
	for _, test := range []struct {
		name     string
		id       StrategyRoutingGraphID
		revision string
	}{
		{name: "zero id", revision: "route-v1"},
		{name: "empty revision", id: 1},
		{name: "non canonical revision", id: 1, revision: "route v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid, identityErr := NewStrategyRoutingGraphIdentity(test.id, test.revision)
			if !errors.Is(identityErr, ErrStrategyRoutingGraphIdentityInvalid) {
				t.Fatalf("error = %v, want invalid identity", identityErr)
			}
			if invalid != (StrategyRoutingGraphIdentity{}) {
				t.Fatalf("failure returned identity: %#v", invalid)
			}
		})
	}
	if !errors.Is((StrategyRoutingGraphIdentity{}).Validate(), ErrStrategyRoutingGraphIdentityInvalid) {
		t.Fatal("zero identity must fail closed")
	}
}

func TestStrategyRoutingGraphRejectsInvalidTopology(t *testing.T) {
	decision1 := mustRoutingDecisionNode(t, 1)
	decision2 := mustRoutingDecisionNode(t, 2)
	target3 := mustRoutingTargetNode(t, 3, 300)
	target4 := mustRoutingTargetNode(t, 4, 400)
	validNodes := []StrategyRoutingNode{decision1, target3, target4}
	validEdges := []StrategyRoutingEdge{
		mustRoutingEdge(t, 1, 3, MembershipRoutingBranchPremiumOverride),
		mustRoutingEdge(t, 1, 4, MembershipRoutingBranchBaselineDefault),
	}
	tests := []struct {
		name  string
		root  StrategyRoutingNodeID
		nodes []StrategyRoutingNode
		edges []StrategyRoutingEdge
		want  error
	}{
		{name: "zero root", nodes: validNodes, edges: validEdges, want: ErrStrategyRoutingGraphRootInvalid},
		{name: "no nodes", root: 1, want: ErrStrategyRoutingGraphRootInvalid},
		{name: "missing root", root: 9, nodes: validNodes, edges: validEdges, want: ErrStrategyRoutingGraphRootInvalid},
		{name: "terminal root", root: 3, nodes: validNodes, edges: validEdges, want: ErrStrategyRoutingGraphRootInvalid},
		{name: "duplicate node id", root: 1, nodes: []StrategyRoutingNode{decision1, decision1, target3, target4}, edges: validEdges, want: ErrStrategyRoutingGraphDuplicateNode},
		{name: "invalid forged node", root: 1, nodes: []StrategyRoutingNode{decision1, {id: 3, kind: "predicate"}, target4}, edges: validEdges, want: ErrStrategyRoutingNodeInvalid},
		{name: "invalid forged edge", root: 1, nodes: validNodes, edges: []StrategyRoutingEdge{{from: 1, to: 3, branch: "premium"}, validEdges[1]}, want: ErrStrategyRoutingEdgeInvalid},
		{name: "dangling source", root: 1, nodes: validNodes, edges: []StrategyRoutingEdge{{from: 9, to: 3, branch: MembershipRoutingBranchPremiumOverride}, validEdges[1]}, want: ErrStrategyRoutingGraphDanglingEdge},
		{name: "dangling destination", root: 1, nodes: validNodes, edges: []StrategyRoutingEdge{mustRoutingEdge(t, 1, 9, MembershipRoutingBranchPremiumOverride), validEdges[1]}, want: ErrStrategyRoutingGraphDanglingEdge},
		{name: "duplicate decision branch", root: 1, nodes: validNodes, edges: []StrategyRoutingEdge{validEdges[0], mustRoutingEdge(t, 1, 4, MembershipRoutingBranchPremiumOverride)}, want: ErrStrategyRoutingGraphDuplicateBranch},
		{name: "decision missing baseline", root: 1, nodes: []StrategyRoutingNode{decision1, target3}, edges: []StrategyRoutingEdge{validEdges[0]}, want: ErrStrategyRoutingEdgeInvalid},
		{name: "terminal has outgoing edge", root: 1, nodes: validNodes, edges: append(append([]StrategyRoutingEdge(nil), validEdges...), mustRoutingEdge(t, 3, 4, MembershipRoutingBranchPremiumOverride)), want: ErrStrategyRoutingEdgeInvalid},
		{name: "unreachable terminal", root: 1, nodes: validNodes, edges: []StrategyRoutingEdge{mustRoutingEdge(t, 1, 3, MembershipRoutingBranchPremiumOverride), mustRoutingEdge(t, 1, 3, MembershipRoutingBranchBaselineDefault)}, want: ErrStrategyRoutingGraphUnreachableNode},
		{
			name:  "cycle wins over unreachable node",
			root:  1,
			nodes: []StrategyRoutingNode{decision1, decision2, target3, target4},
			edges: []StrategyRoutingEdge{
				mustRoutingEdge(t, 1, 2, MembershipRoutingBranchPremiumOverride),
				mustRoutingEdge(t, 1, 3, MembershipRoutingBranchBaselineDefault),
				mustRoutingEdge(t, 2, 1, MembershipRoutingBranchPremiumOverride),
				mustRoutingEdge(t, 2, 3, MembershipRoutingBranchBaselineDefault),
			},
			want: ErrStrategyRoutingGraphCycle,
		},
		{
			name:  "self cycle",
			root:  1,
			nodes: []StrategyRoutingNode{decision1, target3},
			edges: []StrategyRoutingEdge{
				mustRoutingEdge(t, 1, 1, MembershipRoutingBranchPremiumOverride),
				mustRoutingEdge(t, 1, 3, MembershipRoutingBranchBaselineDefault),
			},
			want: ErrStrategyRoutingGraphCycle,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, err := NewStrategyRoutingGraph(1, "route-v1", test.root, test.nodes, test.edges)
			if !errors.Is(err, ErrStrategyRoutingGraphInvalid) || !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want graph invalid and %v", err, test.want)
			}
			assertZeroStrategyRoutingGraph(t, graph)
		})
	}
}

func TestStrategyRoutingGraphEnforcesNodeAndEdgeBudgetsBeforeTraversal(t *testing.T) {
	tooManyNodes := make([]StrategyRoutingNode, MaxStrategyRoutingGraphNodes+1)
	graph, err := NewStrategyRoutingGraph(1, "route-v1", 1, tooManyNodes, nil)
	if !errors.Is(err, ErrStrategyRoutingGraphLimitExceeded) {
		t.Fatalf("node limit error = %v", err)
	}
	assertZeroStrategyRoutingGraph(t, graph)

	tooManyEdges := make([]StrategyRoutingEdge, MaxStrategyRoutingGraphEdges+1)
	graph, err = NewStrategyRoutingGraph(
		1,
		"route-v1",
		1,
		[]StrategyRoutingNode{mustRoutingDecisionNode(t, 1)},
		tooManyEdges,
	)
	if !errors.Is(err, ErrStrategyRoutingGraphLimitExceeded) {
		t.Fatalf("edge limit error = %v", err)
	}
	assertZeroStrategyRoutingGraph(t, graph)
}

func TestStrategyRoutingGraphUsesLongestDepthWhenSharedSuccessorIsVisitedShallowFirst(t *testing.T) {
	nodes := []StrategyRoutingNode{
		mustRoutingTargetNode(t, 5, 500),
		mustRoutingDecisionNode(t, 3),
		mustRoutingDecisionNode(t, 1),
		mustRoutingTargetNode(t, 4, 400),
		mustRoutingDecisionNode(t, 6),
		mustRoutingDecisionNode(t, 2),
	}
	edges := []StrategyRoutingEdge{
		mustRoutingEdge(t, 6, 5, MembershipRoutingBranchBaselineDefault),
		mustRoutingEdge(t, 3, 6, MembershipRoutingBranchPremiumOverride),
		mustRoutingEdge(t, 1, 2, MembershipRoutingBranchBaselineDefault),
		mustRoutingEdge(t, 2, 5, MembershipRoutingBranchBaselineDefault),
		mustRoutingEdge(t, 6, 4, MembershipRoutingBranchPremiumOverride),
		mustRoutingEdge(t, 1, 3, MembershipRoutingBranchPremiumOverride),
		mustRoutingEdge(t, 3, 4, MembershipRoutingBranchBaselineDefault),
		mustRoutingEdge(t, 2, 4, MembershipRoutingBranchPremiumOverride),
	}
	graph, err := NewStrategyRoutingGraph(9, "dag-v1", 1, nodes, edges)
	if err != nil {
		t.Fatalf("construct shared DAG: %v", err)
	}
	if graph.Depth() != 3 {
		t.Fatalf("depth = %d, want longest path depth 3", graph.Depth())
	}
	if got := graph.OutgoingEdges(2); len(got) != 2 || got[0].To() != 5 || got[1].To() != 4 {
		t.Fatalf("unexpected decision 2 branches: %#v", got)
	}
	if got := graph.OutgoingEdges(6); len(got) != 2 || got[0].To() != 5 || got[1].To() != 4 {
		t.Fatalf("shared successors were not preserved: %#v", got)
	}
}

func TestStrategyRoutingGraphDepthBoundaryUsesLongestRootToTerminalPath(t *testing.T) {
	nodes, edges := strategyRoutingChainParts(t, MaxStrategyRoutingGraphDepth)
	graph, err := NewStrategyRoutingGraph(1, "depth-16", 1, nodes, edges)
	if err != nil {
		t.Fatalf("depth boundary should be valid: %v", err)
	}
	if graph.Depth() != MaxStrategyRoutingGraphDepth {
		t.Fatalf("depth = %d, want %d", graph.Depth(), MaxStrategyRoutingGraphDepth)
	}

	nodes, edges = strategyRoutingChainParts(t, MaxStrategyRoutingGraphDepth+1)
	graph, err = NewStrategyRoutingGraph(1, "depth-17", 1, nodes, edges)
	if !errors.Is(err, ErrStrategyRoutingGraphLimitExceeded) {
		t.Fatalf("depth overflow error = %v", err)
	}
	assertZeroStrategyRoutingGraph(t, graph)
}

func TestStrategyRoutingGraphOwnsInputAndOutputSlices(t *testing.T) {
	nodes, edges := minimalStrategyRoutingGraphParts(t)
	graph, err := NewStrategyRoutingGraph(1, "copy-v1", 1, nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	nodes[0] = StrategyRoutingNode{}
	edges[0] = StrategyRoutingEdge{}
	if err := graph.Validate(); err != nil {
		t.Fatalf("caller changed graph through constructor input: %v", err)
	}

	returnedNodes := graph.Nodes()
	returnedEdges := graph.Edges()
	returnedOutgoing := graph.OutgoingEdges(1)
	returnedNodes[0] = StrategyRoutingNode{}
	returnedEdges[0] = StrategyRoutingEdge{}
	returnedOutgoing[0] = StrategyRoutingEdge{}
	if err := graph.Validate(); err != nil {
		t.Fatalf("caller changed graph through accessor output: %v", err)
	}
	if node, found := graph.Node(1); !found || node.RuleCode() != MembershipStrategyRoutingRuleCode {
		t.Fatalf("root changed through a returned slice: %#v, found=%v", node, found)
	}
}

func TestStrategyRoutingGraphReadAccessIsConcurrentAndImmutable(t *testing.T) {
	nodes, edges := strategyRoutingChainParts(t, MaxStrategyRoutingGraphDepth)
	graph, err := NewStrategyRoutingGraph(1, "concurrent-v1", 1, nodes, edges)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			for iteration := 0; iteration < 100; iteration++ {
				if err := graph.Validate(); err != nil {
					t.Errorf("concurrent validate: %v", err)
					return
				}
				if _, found := graph.Node(1); !found {
					t.Error("root disappeared")
					return
				}
				copiedNodes := graph.Nodes()
				copiedEdges := graph.Edges()
				outgoing := graph.OutgoingEdges(1)
				copiedNodes[0] = StrategyRoutingNode{}
				copiedEdges[0] = StrategyRoutingEdge{}
				outgoing[0] = StrategyRoutingEdge{}
			}
		}()
	}
	close(start)
	wait.Wait()
	if err := graph.Validate(); err != nil {
		t.Fatalf("concurrent readers changed graph: %v", err)
	}
}

func TestRestoreStrategyRoutingGraphRoundTripsCanonicalSnapshot(t *testing.T) {
	nodes, edges := strategyRoutingChainParts(t, 3)
	created, err := NewStrategyRoutingGraph(77, "restore-v1", 1, nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreStrategyRoutingGraph(
		created.ID(),
		created.Revision(),
		created.SchemaVersion(),
		created.RootNodeID(),
		created.Nodes(),
		created.Edges(),
	)
	if err != nil {
		t.Fatalf("restore graph: %v", err)
	}
	if restored.ID() != created.ID() || restored.Revision() != created.Revision() ||
		restored.SchemaVersion() != created.SchemaVersion() ||
		restored.RootNodeID() != created.RootNodeID() ||
		restored.Depth() != created.Depth() ||
		!slices.Equal(restored.Nodes(), created.Nodes()) ||
		!slices.Equal(restored.Edges(), created.Edges()) {
		t.Fatalf("restored graph differs: %#v %#v", created, restored)
	}
}

func TestStrategyRoutingGraphValidateRejectsZeroAndForgedDerivedDepth(t *testing.T) {
	if !errors.Is((StrategyRoutingGraph{}).Validate(), ErrStrategyRoutingGraphInvalid) {
		t.Fatal("zero graph must fail closed")
	}
	nodes, edges := minimalStrategyRoutingGraphParts(t)
	valid, err := NewStrategyRoutingGraph(1, "depth-v1", 1, nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	valid.depth++
	if !errors.Is(valid.Validate(), ErrStrategyRoutingGraphInvalid) {
		t.Fatal("forged derived depth must fail validation")
	}
}

func minimalStrategyRoutingGraphParts(t *testing.T) ([]StrategyRoutingNode, []StrategyRoutingEdge) {
	t.Helper()
	return []StrategyRoutingNode{
			mustRoutingDecisionNode(t, 1),
			mustRoutingTargetNode(t, 2, 200),
			mustRoutingTargetNode(t, 3, 300),
		}, []StrategyRoutingEdge{
			mustRoutingEdge(t, 1, 2, MembershipRoutingBranchPremiumOverride),
			mustRoutingEdge(t, 1, 3, MembershipRoutingBranchBaselineDefault),
		}
}

func strategyRoutingChainParts(
	t *testing.T,
	depth int,
) ([]StrategyRoutingNode, []StrategyRoutingEdge) {
	t.Helper()
	if depth < 1 {
		t.Fatalf("test chain depth must be positive: %d", depth)
	}
	targetID := StrategyRoutingNodeID(depth + 1)
	nodes := make([]StrategyRoutingNode, 0, depth+1)
	edges := make([]StrategyRoutingEdge, 0, depth*2)
	for index := 1; index <= depth; index++ {
		nodeID := StrategyRoutingNodeID(index)
		nodes = append(nodes, mustRoutingDecisionNode(t, nodeID))
		premiumTarget := targetID
		if index < depth {
			premiumTarget = StrategyRoutingNodeID(index + 1)
		}
		edges = append(
			edges,
			mustRoutingEdge(t, nodeID, premiumTarget, MembershipRoutingBranchPremiumOverride),
			mustRoutingEdge(t, nodeID, targetID, MembershipRoutingBranchBaselineDefault),
		)
	}
	nodes = append(nodes, mustRoutingTargetNode(t, targetID, 999))
	return nodes, edges
}

func mustRoutingDecisionNode(t *testing.T, id StrategyRoutingNodeID) StrategyRoutingNode {
	t.Helper()
	node, err := NewStrategyRoutingDecisionNode(id, MembershipStrategyRoutingRuleCode)
	if err != nil {
		t.Fatalf("construct decision node %d: %v", id, err)
	}
	return node
}

func mustRoutingTargetNode(
	t *testing.T,
	id StrategyRoutingNodeID,
	strategyID StrategyID,
) StrategyRoutingNode {
	t.Helper()
	node, err := NewStrategyRoutingTargetNode(id, strategyID)
	if err != nil {
		t.Fatalf("construct target node %d: %v", id, err)
	}
	return node
}

func mustRoutingEdge(
	t *testing.T,
	from StrategyRoutingNodeID,
	to StrategyRoutingNodeID,
	branch MembershipRoutingBranch,
) StrategyRoutingEdge {
	t.Helper()
	edge, err := NewStrategyRoutingEdge(from, to, branch)
	if err != nil {
		t.Fatalf("construct edge %d -> %d (%q): %v", from, to, branch, err)
	}
	return edge
}

func assertZeroStrategyRoutingGraph(t *testing.T, graph StrategyRoutingGraph) {
	t.Helper()
	if graph.ID() != 0 || graph.Revision() != "" || graph.SchemaVersion() != 0 ||
		graph.RootNodeID() != 0 || graph.Depth() != 0 || len(graph.Nodes()) != 0 ||
		len(graph.Edges()) != 0 {
		t.Fatalf("failure returned graph state: %#v", graph)
	}
}
