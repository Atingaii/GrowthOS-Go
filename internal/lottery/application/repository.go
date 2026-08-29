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
