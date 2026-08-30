package application

import (
	"context"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

// StrategyCreator is the smallest write port required by the current use case.
// It intentionally does not imply update, upsert, delete, or list semantics.
type StrategyCreator interface {
	Create(ctx context.Context, strategy domain.Strategy) error
}

// StrategyReader is the smallest read port required by the current use case.
type StrategyReader interface {
	FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error)
}

// StrategySnapshotCreator is the create-only write port for one complete,
// immutable Strategy configuration revision. It deliberately exposes no
// update, upsert, delete, publish, or latest-revision operation.
type StrategySnapshotCreator interface {
	CreateSnapshot(ctx context.Context, snapshot domain.StrategySnapshot) error
}

// StrategySnapshotReader restores exactly one immutable Strategy snapshot by
// its validated StrategyID/revision identity. StrategyID-only reads keep their
// existing non-versioned semantics and must not be used as a substitute.
type StrategySnapshotReader interface {
	FindSnapshotByIdentity(
		ctx context.Context,
		identity domain.StrategySnapshotIdentity,
	) (domain.StrategySnapshot, error)
}

// StrategyRoutingGraphCreator is the create-only write port for one complete
// immutable graph revision. Implementations must reject invalid aggregates
// before storage. The port intentionally exposes no update, upsert, delete,
// publish, or partial node/edge mutation operation.
type StrategyRoutingGraphCreator interface {
	Create(ctx context.Context, graph domain.StrategyRoutingGraph) error
}

// StrategyRoutingGraphReader restores exactly one immutable graph snapshot by
// its validated GraphID/revision identity. It does not imply latest-revision,
// list, publication, traversal, or Strategy loading semantics.
type StrategyRoutingGraphReader interface {
	FindByIdentity(
		ctx context.Context,
		identity domain.StrategyRoutingGraphIdentity,
	) (domain.StrategyRoutingGraph, error)
}
