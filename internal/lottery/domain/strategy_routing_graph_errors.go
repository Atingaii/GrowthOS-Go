package domain

import "errors"

var (
	// ErrStrategyRoutingGraphInvalid is the stable fail-closed classification for
	// a graph snapshot that cannot safely become Lottery routing state.
	ErrStrategyRoutingGraphInvalid = errors.New("lottery: strategy routing graph is invalid")
	// ErrStrategyRoutingGraphIdentityInvalid reports a missing graph identity or
	// a revision outside the canonical v1 ASCII grammar.
	ErrStrategyRoutingGraphIdentityInvalid = errors.New("lottery: strategy routing graph identity is invalid")
	// ErrStrategyRoutingGraphSchemaUnsupported reports a zero, future, or
	// otherwise unknown persisted schema version.
	ErrStrategyRoutingGraphSchemaUnsupported = errors.New("lottery: strategy routing graph schema is unsupported")
	// ErrStrategyRoutingNodeInvalid reports an invalid discriminated node value.
	ErrStrategyRoutingNodeInvalid = errors.New("lottery: strategy routing node is invalid")
	// ErrStrategyRoutingEdgeInvalid reports an invalid concrete branch edge.
	ErrStrategyRoutingEdgeInvalid = errors.New("lottery: strategy routing edge is invalid")
	// ErrStrategyRoutingGraphLimitExceeded reports a snapshot beyond the bounded
	// node, edge, or longest-path depth budget.
	ErrStrategyRoutingGraphLimitExceeded = errors.New("lottery: strategy routing graph limit exceeded")
	// ErrStrategyRoutingGraphDuplicateNode reports repeated node identity.
	ErrStrategyRoutingGraphDuplicateNode = errors.New("lottery: strategy routing graph node id is duplicated")
	// ErrStrategyRoutingGraphDuplicateBranch reports repeated branch identity on
	// one decision node.
	ErrStrategyRoutingGraphDuplicateBranch = errors.New("lottery: strategy routing graph decision branch is duplicated")
	// ErrStrategyRoutingGraphDanglingEdge reports an edge whose endpoint is not
	// present in the same immutable graph snapshot.
	ErrStrategyRoutingGraphDanglingEdge = errors.New("lottery: strategy routing graph edge is dangling")
	// ErrStrategyRoutingGraphRootInvalid reports a missing or non-decision root.
	ErrStrategyRoutingGraphRootInvalid = errors.New("lottery: strategy routing graph root is invalid")
	// ErrStrategyRoutingGraphUnreachableNode reports state outside the rooted
	// graph. Such state is rejected rather than silently ignored.
	ErrStrategyRoutingGraphUnreachableNode = errors.New("lottery: strategy routing graph node is unreachable")
	// ErrStrategyRoutingGraphCycle reports a cycle in a snapshot that must be a
	// finite rooted DAG.
	ErrStrategyRoutingGraphCycle = errors.New("lottery: strategy routing graph contains a cycle")
)
