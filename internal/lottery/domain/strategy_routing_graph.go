package domain

import (
	"cmp"
	"fmt"
	"slices"
)

const (
	// MaxStrategyRoutingGraphRevisionBytes bounds the persisted revision token.
	// Its v1 grammar is [A-Za-z0-9][A-Za-z0-9._:-]{0,127}.
	MaxStrategyRoutingGraphRevisionBytes = 128
	// MaxStrategyRoutingGraphNodes bounds validation, recovery, and later
	// traversal work for one immutable snapshot.
	MaxStrategyRoutingGraphNodes = 128
	// MaxStrategyRoutingGraphEdges bounds total adjacency in one snapshot.
	MaxStrategyRoutingGraphEdges = 256
	// MaxStrategyRoutingGraphDepth is the maximum number of edges on the longest
	// root-to-terminal path. A decision directly targeting a Strategy has depth 1.
	MaxStrategyRoutingGraphDepth = 16
)

// StrategyRoutingGraphID identifies one durable Lottery-owned routing graph.
type StrategyRoutingGraphID uint64

// StrategyRoutingGraphRevision identifies one immutable graph snapshot. It is
// a bounded correlation token, not by itself a content hash.
type StrategyRoutingGraphRevision string

// StrategyRoutingGraphIdentity is the narrow validated lookup identity shared
// by graph owners and later repository ports. Revision remains a correlation
// token; this value object does not claim content-addressed uniqueness.
type StrategyRoutingGraphIdentity struct {
	id       StrategyRoutingGraphID
	revision StrategyRoutingGraphRevision
}

// NewStrategyRoutingGraphIdentity constructs a canonical GraphID/revision pair.
func NewStrategyRoutingGraphIdentity(
	id StrategyRoutingGraphID,
	revision string,
) (StrategyRoutingGraphIdentity, error) {
	identity := StrategyRoutingGraphIdentity{
		id:       id,
		revision: StrategyRoutingGraphRevision(revision),
	}
	if err := identity.Validate(); err != nil {
		return StrategyRoutingGraphIdentity{}, err
	}
	return identity, nil
}

// Validate rejects zero identity and revisions outside the v1 ASCII grammar.
func (identity StrategyRoutingGraphIdentity) Validate() error {
	if identity.id == 0 {
		return fmt.Errorf("%w: graph id is required", ErrStrategyRoutingGraphIdentityInvalid)
	}
	if err := validateStrategyRoutingGraphRevision(identity.revision); err != nil {
		return fmt.Errorf("%w: revision %v", ErrStrategyRoutingGraphIdentityInvalid, err)
	}
	return nil
}

// ID returns the durable graph identity.
func (identity StrategyRoutingGraphIdentity) ID() StrategyRoutingGraphID { return identity.id }

// Revision returns the bounded snapshot correlation token. The token alone is
// neither a content hash nor a registry-backed uniqueness guarantee.
func (identity StrategyRoutingGraphIdentity) Revision() StrategyRoutingGraphRevision {
	return identity.revision
}

// StrategyRoutingGraphSchemaVersion identifies the closed persisted shape.
type StrategyRoutingGraphSchemaVersion uint16

const (
	// StrategyRoutingGraphSchemaVersionV1 is the only graph shape this lesson can
	// construct or restore. Unknown versions fail closed.
	StrategyRoutingGraphSchemaVersionV1 StrategyRoutingGraphSchemaVersion = 1
)

// StrategyRoutingNodeID identifies one node within a graph snapshot.
type StrategyRoutingNodeID uint64

// StrategyRoutingNodeKind is the closed v1 node discriminant.
type StrategyRoutingNodeKind string

const (
	StrategyRoutingNodeKindDecision       StrategyRoutingNodeKind = "decision"
	StrategyRoutingNodeKindStrategyTarget StrategyRoutingNodeKind = "strategy_target"
)

// StrategyRoutingNode is one immutable discriminated node. Decision nodes own
// a concrete Lottery rule code; terminal nodes own a Strategy identity. The
// inactive variant is always zero.
type StrategyRoutingNode struct {
	id         StrategyRoutingNodeID
	kind       StrategyRoutingNodeKind
	ruleCode   MembershipRoutingRuleCode
	strategyID StrategyID
}

// NewStrategyRoutingDecisionNode constructs the only approved v1 decision
// node. It deliberately does not accept an expression or generic condition.
func NewStrategyRoutingDecisionNode(
	id StrategyRoutingNodeID,
	ruleCode MembershipRoutingRuleCode,
) (StrategyRoutingNode, error) {
	return restoreStrategyRoutingNode(id, StrategyRoutingNodeKindDecision, ruleCode, 0)
}

// NewStrategyRoutingTargetNode constructs a terminal Strategy reference. It
// does not load or select from the Strategy aggregate.
func NewStrategyRoutingTargetNode(
	id StrategyRoutingNodeID,
	strategyID StrategyID,
) (StrategyRoutingNode, error) {
	return restoreStrategyRoutingNode(id, StrategyRoutingNodeKindStrategyTarget, "", strategyID)
}

// RestoreStrategyRoutingNode reconstructs a persisted discriminated node and
// rejects mixed, zero, or unknown variants without normalization.
func RestoreStrategyRoutingNode(
	id StrategyRoutingNodeID,
	kind StrategyRoutingNodeKind,
	ruleCode MembershipRoutingRuleCode,
	strategyID StrategyID,
) (StrategyRoutingNode, error) {
	return restoreStrategyRoutingNode(id, kind, ruleCode, strategyID)
}

func restoreStrategyRoutingNode(
	id StrategyRoutingNodeID,
	kind StrategyRoutingNodeKind,
	ruleCode MembershipRoutingRuleCode,
	strategyID StrategyID,
) (StrategyRoutingNode, error) {
	node := StrategyRoutingNode{
		id:         id,
		kind:       kind,
		ruleCode:   ruleCode,
		strategyID: strategyID,
	}
	if err := node.Validate(); err != nil {
		return StrategyRoutingNode{}, err
	}
	return node, nil
}

// Validate checks the closed node union and the one approved decision rule.
func (node StrategyRoutingNode) Validate() error {
	if node.id == 0 {
		return fmt.Errorf("%w: node id is required", ErrStrategyRoutingNodeInvalid)
	}
	switch node.kind {
	case StrategyRoutingNodeKindDecision:
		if node.ruleCode != MembershipStrategyRoutingRuleCode {
			return fmt.Errorf("%w: decision rule code is unsupported", ErrStrategyRoutingNodeInvalid)
		}
		if node.strategyID != 0 {
			return fmt.Errorf("%w: decision cannot contain a Strategy target", ErrStrategyRoutingNodeInvalid)
		}
	case StrategyRoutingNodeKindStrategyTarget:
		if node.ruleCode != "" {
			return fmt.Errorf("%w: terminal cannot contain a rule code", ErrStrategyRoutingNodeInvalid)
		}
		if node.strategyID == 0 {
			return fmt.Errorf("%w: terminal Strategy target is required", ErrStrategyRoutingNodeInvalid)
		}
	default:
		return fmt.Errorf("%w: node kind is unsupported", ErrStrategyRoutingNodeInvalid)
	}
	return nil
}

// ID returns the identity unique within the enclosing graph snapshot.
func (node StrategyRoutingNode) ID() StrategyRoutingNodeID { return node.id }

// Kind returns the closed decision or strategy_target discriminant.
func (node StrategyRoutingNode) Kind() StrategyRoutingNodeKind { return node.kind }

// RuleCode returns the decision rule code, or zero for a terminal node.
func (node StrategyRoutingNode) RuleCode() MembershipRoutingRuleCode { return node.ruleCode }

// StrategyID returns the terminal target, or zero for a decision node.
func (node StrategyRoutingNode) StrategyID() StrategyID { return node.strategyID }

// StrategyRoutingEdge is one immutable, directed decision branch. isDefault is
// persisted evidence and must agree with the closed branch vocabulary.
type StrategyRoutingEdge struct {
	from      StrategyRoutingNodeID
	to        StrategyRoutingNodeID
	branch    MembershipRoutingBranch
	isDefault bool
}

// NewStrategyRoutingEdge constructs an approved v1 branch and derives its
// canonical default marker. Only baseline_default is the default edge.
func NewStrategyRoutingEdge(
	from StrategyRoutingNodeID,
	to StrategyRoutingNodeID,
	branch MembershipRoutingBranch,
) (StrategyRoutingEdge, error) {
	return RestoreStrategyRoutingEdge(
		from,
		to,
		branch,
		branch == MembershipRoutingBranchBaselineDefault,
	)
}

// RestoreStrategyRoutingEdge checks the persisted default marker instead of
// silently rewriting it during recovery.
func RestoreStrategyRoutingEdge(
	from StrategyRoutingNodeID,
	to StrategyRoutingNodeID,
	branch MembershipRoutingBranch,
	isDefault bool,
) (StrategyRoutingEdge, error) {
	edge := StrategyRoutingEdge{
		from:      from,
		to:        to,
		branch:    branch,
		isDefault: isDefault,
	}
	if err := edge.Validate(); err != nil {
		return StrategyRoutingEdge{}, err
	}
	return edge, nil
}

// Validate rejects zero endpoints, unknown branches, and mismatched defaults.
func (edge StrategyRoutingEdge) Validate() error {
	if edge.from == 0 || edge.to == 0 {
		return fmt.Errorf("%w: both endpoints are required", ErrStrategyRoutingEdgeInvalid)
	}
	switch edge.branch {
	case MembershipRoutingBranchPremiumOverride:
		if edge.isDefault {
			return fmt.Errorf("%w: premium override cannot be default", ErrStrategyRoutingEdgeInvalid)
		}
	case MembershipRoutingBranchBaselineDefault:
		if !edge.isDefault {
			return fmt.Errorf("%w: baseline branch must be default", ErrStrategyRoutingEdgeInvalid)
		}
	default:
		return fmt.Errorf("%w: branch is unsupported", ErrStrategyRoutingEdgeInvalid)
	}
	return nil
}

// From returns the source decision node identity.
func (edge StrategyRoutingEdge) From() StrategyRoutingNodeID { return edge.from }

// To returns the successor node identity.
func (edge StrategyRoutingEdge) To() StrategyRoutingNodeID { return edge.to }

// Branch returns the concrete membership branch identity.
func (edge StrategyRoutingEdge) Branch() MembershipRoutingBranch { return edge.branch }

// IsDefault reports whether this is the required baseline fallback edge.
func (edge StrategyRoutingEdge) IsDefault() bool { return edge.isDefault }

// StrategyRoutingGraph is one bounded immutable rooted DAG. It defines safe
// topology only; it deliberately contains no evaluator, expression language,
// generic value bag, runtime publication state, or loaded Strategy aggregates.
type StrategyRoutingGraph struct {
	id            StrategyRoutingGraphID
	revision      StrategyRoutingGraphRevision
	schemaVersion StrategyRoutingGraphSchemaVersion
	rootNodeID    StrategyRoutingNodeID
	nodes         []StrategyRoutingNode
	edges         []StrategyRoutingEdge
	depth         uint16
}

// NewStrategyRoutingGraph constructs the current code-owned schema. Collection
// order is canonicalized, while scalar values such as revision are never
// silently trimmed or rewritten.
func NewStrategyRoutingGraph(
	id StrategyRoutingGraphID,
	revision string,
	rootNodeID StrategyRoutingNodeID,
	nodes []StrategyRoutingNode,
	edges []StrategyRoutingEdge,
) (StrategyRoutingGraph, error) {
	return restoreStrategyRoutingGraph(
		id,
		StrategyRoutingGraphRevision(revision),
		StrategyRoutingGraphSchemaVersionV1,
		rootNodeID,
		nodes,
		edges,
	)
}

// RestoreStrategyRoutingGraph reconstructs a persisted snapshot. Unlike New,
// it accepts a stored schema marker so zero and future versions fail closed.
func RestoreStrategyRoutingGraph(
	id StrategyRoutingGraphID,
	revision StrategyRoutingGraphRevision,
	schemaVersion StrategyRoutingGraphSchemaVersion,
	rootNodeID StrategyRoutingNodeID,
	nodes []StrategyRoutingNode,
	edges []StrategyRoutingEdge,
) (StrategyRoutingGraph, error) {
	return restoreStrategyRoutingGraph(id, revision, schemaVersion, rootNodeID, nodes, edges)
}

func restoreStrategyRoutingGraph(
	id StrategyRoutingGraphID,
	revision StrategyRoutingGraphRevision,
	schemaVersion StrategyRoutingGraphSchemaVersion,
	rootNodeID StrategyRoutingNodeID,
	nodes []StrategyRoutingNode,
	edges []StrategyRoutingEdge,
) (StrategyRoutingGraph, error) {
	canonicalNodes, canonicalEdges, depth, err := validateStrategyRoutingGraphState(
		id,
		revision,
		schemaVersion,
		rootNodeID,
		nodes,
		edges,
	)
	if err != nil {
		return StrategyRoutingGraph{}, err
	}
	return StrategyRoutingGraph{
		id:            id,
		revision:      revision,
		schemaVersion: schemaVersion,
		rootNodeID:    rootNodeID,
		nodes:         canonicalNodes,
		edges:         canonicalEdges,
		depth:         uint16(depth),
	}, nil
}

// Validate rechecks the complete immutable snapshot, including its derived
// longest-path depth. The zero value and manually forged state fail closed.
func (graph StrategyRoutingGraph) Validate() error {
	_, _, depth, err := validateStrategyRoutingGraphState(
		graph.id,
		graph.revision,
		graph.schemaVersion,
		graph.rootNodeID,
		graph.nodes,
		graph.edges,
	)
	if err != nil {
		return err
	}
	if int(graph.depth) != depth {
		return invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphInvalid,
			"stored depth does not match topology",
		)
	}
	return nil
}

// ID returns the durable graph identity.
func (graph StrategyRoutingGraph) ID() StrategyRoutingGraphID { return graph.id }

// Identity returns the validated GraphID/revision lookup pair.
func (graph StrategyRoutingGraph) Identity() StrategyRoutingGraphIdentity {
	return StrategyRoutingGraphIdentity{id: graph.id, revision: graph.revision}
}

// Revision returns this snapshot's bounded correlation token. The token alone
// is neither a content hash nor a registry-backed uniqueness guarantee.
func (graph StrategyRoutingGraph) Revision() StrategyRoutingGraphRevision {
	return graph.revision
}

// SchemaVersion returns the closed persisted shape marker.
func (graph StrategyRoutingGraph) SchemaVersion() StrategyRoutingGraphSchemaVersion {
	return graph.schemaVersion
}

// RootNodeID returns the aggregate-owned entry node identity.
func (graph StrategyRoutingGraph) RootNodeID() StrategyRoutingNodeID {
	return graph.rootNodeID
}

// Depth returns the longest root-to-terminal path measured in edges.
func (graph StrategyRoutingGraph) Depth() int { return int(graph.depth) }

// Nodes returns a defensive copy in ascending node-ID order.
func (graph StrategyRoutingGraph) Nodes() []StrategyRoutingNode {
	return append([]StrategyRoutingNode(nil), graph.nodes...)
}

// Edges returns a defensive copy in source, branch, destination order.
func (graph StrategyRoutingGraph) Edges() []StrategyRoutingEdge {
	return append([]StrategyRoutingEdge(nil), graph.edges...)
}

// Node returns one node by identity without exposing mutable graph storage.
func (graph StrategyRoutingGraph) Node(id StrategyRoutingNodeID) (StrategyRoutingNode, bool) {
	index, found := slices.BinarySearchFunc(
		graph.nodes,
		id,
		func(node StrategyRoutingNode, target StrategyRoutingNodeID) int {
			return cmp.Compare(node.id, target)
		},
	)
	if !found {
		return StrategyRoutingNode{}, false
	}
	return graph.nodes[index], true
}

// OutgoingEdges returns a defensive copy in canonical branch order. Terminal
// and unknown nodes return an empty slice.
func (graph StrategyRoutingGraph) OutgoingEdges(id StrategyRoutingNodeID) []StrategyRoutingEdge {
	first, _ := slices.BinarySearchFunc(
		graph.edges,
		id,
		func(edge StrategyRoutingEdge, target StrategyRoutingNodeID) int {
			return cmp.Compare(edge.from, target)
		},
	)
	result := make([]StrategyRoutingEdge, 0, 2)
	for index := first; index < len(graph.edges) && graph.edges[index].from == id; index++ {
		result = append(result, graph.edges[index])
	}
	return result
}

func validateStrategyRoutingGraphState(
	id StrategyRoutingGraphID,
	revision StrategyRoutingGraphRevision,
	schemaVersion StrategyRoutingGraphSchemaVersion,
	rootNodeID StrategyRoutingNodeID,
	nodes []StrategyRoutingNode,
	edges []StrategyRoutingEdge,
) ([]StrategyRoutingNode, []StrategyRoutingEdge, int, error) {
	identity := StrategyRoutingGraphIdentity{id: id, revision: revision}
	if err := identity.Validate(); err != nil {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphIdentityInvalid,
			"identity %v",
			err,
		)
	}
	if schemaVersion != StrategyRoutingGraphSchemaVersionV1 {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphSchemaUnsupported,
			"schema version %d is unsupported",
			schemaVersion,
		)
	}
	if rootNodeID == 0 {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphRootInvalid,
			"root node id is required",
		)
	}
	if len(nodes) == 0 {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphRootInvalid,
			"nodes are required",
		)
	}
	if len(nodes) > MaxStrategyRoutingGraphNodes {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphLimitExceeded,
			"node count %d exceeds %d",
			len(nodes),
			MaxStrategyRoutingGraphNodes,
		)
	}
	if len(edges) > MaxStrategyRoutingGraphEdges {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphLimitExceeded,
			"edge count %d exceeds %d",
			len(edges),
			MaxStrategyRoutingGraphEdges,
		)
	}

	canonicalNodes := append([]StrategyRoutingNode(nil), nodes...)
	slices.SortFunc(canonicalNodes, func(left, right StrategyRoutingNode) int {
		return cmp.Compare(left.id, right.id)
	})
	nodeByID := make(map[StrategyRoutingNodeID]StrategyRoutingNode, len(canonicalNodes))
	for index, node := range canonicalNodes {
		if err := node.Validate(); err != nil {
			return nil, nil, 0, invalidStrategyRoutingGraph(
				ErrStrategyRoutingNodeInvalid,
				"node at canonical index %d: %v",
				index,
				err,
			)
		}
		if _, exists := nodeByID[node.id]; exists {
			return nil, nil, 0, invalidStrategyRoutingGraph(
				ErrStrategyRoutingGraphDuplicateNode,
				"node id %d",
				node.id,
			)
		}
		nodeByID[node.id] = node
	}
	root, exists := nodeByID[rootNodeID]
	if !exists || root.kind != StrategyRoutingNodeKindDecision {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphRootInvalid,
			"root %d must identify a decision node",
			rootNodeID,
		)
	}

	canonicalEdges := append([]StrategyRoutingEdge(nil), edges...)
	slices.SortFunc(canonicalEdges, compareStrategyRoutingEdges)
	outgoing := make(map[StrategyRoutingNodeID][]StrategyRoutingEdge, len(canonicalNodes))
	for index, edge := range canonicalEdges {
		if err := edge.Validate(); err != nil {
			return nil, nil, 0, invalidStrategyRoutingGraph(
				ErrStrategyRoutingEdgeInvalid,
				"edge at canonical index %d: %v",
				index,
				err,
			)
		}
		if _, exists := nodeByID[edge.from]; !exists {
			return nil, nil, 0, invalidStrategyRoutingGraph(
				ErrStrategyRoutingGraphDanglingEdge,
				"source node %d is missing",
				edge.from,
			)
		}
		if _, exists := nodeByID[edge.to]; !exists {
			return nil, nil, 0, invalidStrategyRoutingGraph(
				ErrStrategyRoutingGraphDanglingEdge,
				"destination node %d is missing",
				edge.to,
			)
		}
		branches := outgoing[edge.from]
		if len(branches) > 0 && branches[len(branches)-1].branch == edge.branch {
			return nil, nil, 0, invalidStrategyRoutingGraph(
				ErrStrategyRoutingGraphDuplicateBranch,
				"node %d branch %q",
				edge.from,
				edge.branch,
			)
		}
		outgoing[edge.from] = append(branches, edge)
	}

	for _, node := range canonicalNodes {
		branches := outgoing[node.id]
		switch node.kind {
		case StrategyRoutingNodeKindDecision:
			if err := validateStrategyRoutingDecisionBranches(branches); err != nil {
				return nil, nil, 0, invalidStrategyRoutingGraph(
					ErrStrategyRoutingEdgeInvalid,
					"decision node %d: %v",
					node.id,
					err,
				)
			}
		case StrategyRoutingNodeKindStrategyTarget:
			if len(branches) != 0 {
				return nil, nil, 0, invalidStrategyRoutingGraph(
					ErrStrategyRoutingEdgeInvalid,
					"terminal node %d has outgoing edges",
					node.id,
				)
			}
		}
	}

	depth, reachableCount, err := strategyRoutingGraphDepth(rootNodeID, nodeByID, outgoing)
	if err != nil {
		return nil, nil, 0, err
	}
	if reachableCount != len(canonicalNodes) {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphUnreachableNode,
			"%d of %d nodes are reachable",
			reachableCount,
			len(canonicalNodes),
		)
	}
	if depth > MaxStrategyRoutingGraphDepth {
		return nil, nil, 0, invalidStrategyRoutingGraph(
			ErrStrategyRoutingGraphLimitExceeded,
			"depth %d exceeds %d",
			depth,
			MaxStrategyRoutingGraphDepth,
		)
	}
	return canonicalNodes, canonicalEdges, depth, nil
}

func validateStrategyRoutingGraphRevision(revision StrategyRoutingGraphRevision) error {
	value := string(revision)
	if len(value) == 0 {
		return fmt.Errorf("revision is required")
	}
	if len(value) > MaxStrategyRoutingGraphRevisionBytes {
		return fmt.Errorf("revision exceeds %d bytes", MaxStrategyRoutingGraphRevisionBytes)
	}
	for index, character := range []byte(value) {
		if isASCIILetterOrDigit(character) {
			continue
		}
		if index > 0 {
			switch character {
			case '.', '_', ':', '-':
				continue
			}
		}
		return fmt.Errorf("revision does not match the v1 ASCII token grammar")
	}
	return nil
}

func isASCIILetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}

func compareStrategyRoutingEdges(left, right StrategyRoutingEdge) int {
	if result := cmp.Compare(left.from, right.from); result != 0 {
		return result
	}
	if result := cmp.Compare(left.branch, right.branch); result != 0 {
		return result
	}
	if result := cmp.Compare(left.to, right.to); result != 0 {
		return result
	}
	if left.isDefault == right.isDefault {
		return 0
	}
	if !left.isDefault {
		return -1
	}
	return 1
}

func validateStrategyRoutingDecisionBranches(edges []StrategyRoutingEdge) error {
	if len(edges) != 2 {
		return fmt.Errorf("exactly two approved branches are required")
	}
	var premium, baseline int
	for _, edge := range edges {
		switch edge.branch {
		case MembershipRoutingBranchPremiumOverride:
			premium++
			if edge.isDefault {
				return fmt.Errorf("premium override cannot be default")
			}
		case MembershipRoutingBranchBaselineDefault:
			baseline++
			if !edge.isDefault {
				return fmt.Errorf("baseline branch must be default")
			}
		default:
			return fmt.Errorf("branch %q is unsupported", edge.branch)
		}
	}
	if premium != 1 || baseline != 1 {
		return fmt.Errorf("one premium override and one baseline default are required")
	}
	return nil
}

func strategyRoutingGraphDepth(
	rootNodeID StrategyRoutingNodeID,
	nodeByID map[StrategyRoutingNodeID]StrategyRoutingNode,
	outgoing map[StrategyRoutingNodeID][]StrategyRoutingEdge,
) (int, int, error) {
	const (
		unvisited uint8 = iota
		visiting
		visited
	)
	state := make(map[StrategyRoutingNodeID]uint8, len(nodeByID))
	depthByNode := make(map[StrategyRoutingNodeID]int, len(nodeByID))
	var visit func(StrategyRoutingNodeID) (int, error)
	visit = func(nodeID StrategyRoutingNodeID) (int, error) {
		switch state[nodeID] {
		case visiting:
			return 0, invalidStrategyRoutingGraph(
				ErrStrategyRoutingGraphCycle,
				"cycle reaches node %d",
				nodeID,
			)
		case visited:
			return depthByNode[nodeID], nil
		}
		state[nodeID] = visiting
		longest := 0
		for _, edge := range outgoing[nodeID] {
			childDepth, err := visit(edge.to)
			if err != nil {
				return 0, err
			}
			longest = max(longest, childDepth+1)
		}
		state[nodeID] = visited
		depthByNode[nodeID] = longest
		return longest, nil
	}
	depth, err := visit(rootNodeID)
	if err != nil {
		return 0, 0, err
	}
	return depth, len(state), nil
}

func invalidStrategyRoutingGraph(cause error, format string, arguments ...any) error {
	detail := fmt.Sprintf(format, arguments...)
	return fmt.Errorf("%w: %w: %s", ErrStrategyRoutingGraphInvalid, cause, detail)
}
