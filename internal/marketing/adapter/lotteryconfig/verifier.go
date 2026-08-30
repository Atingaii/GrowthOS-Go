package lotteryconfig

import (
	"context"
	"errors"
	"reflect"

	lotteryapplication "github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	lotterydomain "github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	marketingapplication "github.com/Atingaii/GrowthOS-Go/internal/marketing/application"
)

// Verifier is the only v1 cross-context adapter. It translates exact primitive
// references at the boundary, then asks Lottery-owned repositories for exact
// immutable snapshots.
type Verifier struct {
	graphs     lotteryapplication.StrategyRoutingGraphReader
	strategies lotteryapplication.StrategySnapshotReader
}

// NewVerifier rejects nil and typed-nil Lottery ports.
func NewVerifier(
	graphs lotteryapplication.StrategyRoutingGraphReader,
	strategies lotteryapplication.StrategySnapshotReader,
) (*Verifier, error) {
	verifier := &Verifier{graphs: graphs, strategies: strategies}
	if err := verifier.Validate(); err != nil {
		return nil, err
	}
	return verifier, nil
}

// Validate reports whether both exact readers are usable.
func (verifier *Verifier) Validate() error {
	if verifier == nil || dependencyIsNil(verifier.graphs) || dependencyIsNil(verifier.strategies) {
		return marketingapplication.WrapLotteryVerificationError(
			marketingapplication.ErrLotteryPublicationUnavailable,
			errors.New("exact Lottery readers are not configured"),
		)
	}
	return nil
}

// VerifyPublication proves the closed exact configuration set. Any failure is
// returned without a partial graph, terminal set, or Strategy snapshot.
func (verifier *Verifier) VerifyPublication(
	ctx context.Context,
	candidate marketingapplication.ActivityPublicationCandidate,
) error {
	if ctx == nil {
		return marketingapplication.WrapLotteryVerificationError(
			marketingapplication.ErrLotteryPublicationInvalid,
			errors.New("context is required"),
		)
	}
	if err := candidate.Validate(); err != nil {
		return marketingapplication.WrapLotteryVerificationError(
			marketingapplication.ErrLotteryPublicationInvalid,
			err,
		)
	}
	if err := verifier.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	graphReference := candidate.GraphReference()
	graphIdentity, err := lotterydomain.NewStrategyRoutingGraphIdentity(
		lotterydomain.StrategyRoutingGraphID(graphReference.ID()),
		string(graphReference.Revision()),
	)
	if err != nil {
		return marketingapplication.WrapLotteryVerificationError(
			marketingapplication.ErrLotteryPublicationInvalid,
			err,
		)
	}
	graph, dependencyErr := verifier.graphs.FindByIdentity(ctx, graphIdentity)
	if err := ctx.Err(); err != nil {
		return err
	}
	if dependencyErr != nil {
		return classifyGraphReadError(dependencyErr)
	}
	if graph.Identity() != graphIdentity {
		return marketingapplication.WrapLotteryVerificationError(
			marketingapplication.ErrLotteryPublicationInvalid,
			errors.New("graph reader returned a different exact identity"),
		)
	}
	if err := graph.Validate(); err != nil {
		return marketingapplication.WrapLotteryVerificationError(
			marketingapplication.ErrLotteryPublicationInvalid,
			err,
		)
	}

	terminalStrategies := make(map[lotterydomain.StrategyID]struct{})
	for _, node := range graph.Nodes() {
		if node.Kind() != lotterydomain.StrategyRoutingNodeKindStrategyTarget {
			continue
		}
		strategyID := node.StrategyID()
		if strategyID == 0 {
			return marketingapplication.WrapLotteryVerificationError(
				marketingapplication.ErrLotteryPublicationInvalid,
				errors.New("graph contains a zero terminal Strategy identity"),
			)
		}
		terminalStrategies[strategyID] = struct{}{}
	}

	manifest := candidate.StrategyRevisionManifest()
	if len(terminalStrategies) != len(manifest) {
		return marketingapplication.WrapLotteryVerificationError(
			marketingapplication.ErrLotteryPublicationInvalid,
			errors.New("terminal and manifest Strategy sets differ"),
		)
	}
	for _, reference := range manifest {
		strategyID := lotterydomain.StrategyID(reference.StrategyID())
		if _, exists := terminalStrategies[strategyID]; !exists {
			return marketingapplication.WrapLotteryVerificationError(
				marketingapplication.ErrLotteryPublicationInvalid,
				errors.New("manifest contains a Strategy outside the graph terminal set"),
			)
		}

		identity, err := lotterydomain.NewStrategySnapshotIdentity(
			strategyID,
			string(reference.Revision()),
		)
		if err != nil {
			return marketingapplication.WrapLotteryVerificationError(
				marketingapplication.ErrLotteryPublicationInvalid,
				err,
			)
		}
		snapshot, dependencyErr := verifier.strategies.FindSnapshotByIdentity(ctx, identity)
		if err := ctx.Err(); err != nil {
			return err
		}
		if dependencyErr != nil {
			return classifyStrategyReadError(dependencyErr)
		}
		if snapshot.Identity() != identity {
			return marketingapplication.WrapLotteryVerificationError(
				marketingapplication.ErrLotteryPublicationInvalid,
				errors.New("Strategy reader returned a different exact identity"),
			)
		}
		if err := snapshot.Validate(); err != nil {
			return marketingapplication.WrapLotteryVerificationError(
				marketingapplication.ErrLotteryPublicationInvalid,
				err,
			)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func classifyGraphReadError(err error) error {
	class := marketingapplication.ErrLotteryPublicationUnavailable
	if errors.Is(err, lotteryapplication.ErrStrategyRoutingGraphNotFound) ||
		errors.Is(err, lotteryapplication.ErrStoredStrategyRoutingGraphInvalid) ||
		errors.Is(err, lotterydomain.ErrStrategyRoutingGraphInvalid) ||
		errors.Is(err, lotterydomain.ErrStrategyRoutingGraphIdentityInvalid) ||
		errors.Is(err, lotterydomain.ErrStrategyRoutingGraphSchemaUnsupported) {
		class = marketingapplication.ErrLotteryPublicationInvalid
	}
	return marketingapplication.WrapLotteryVerificationError(class, err)
}

func classifyStrategyReadError(err error) error {
	class := marketingapplication.ErrLotteryPublicationUnavailable
	if errors.Is(err, lotteryapplication.ErrStrategySnapshotNotFound) ||
		errors.Is(err, lotteryapplication.ErrStoredStrategySnapshotInvalid) ||
		errors.Is(err, lotterydomain.ErrStrategySnapshotInvalid) ||
		errors.Is(err, lotterydomain.ErrStrategySnapshotIdentityInvalid) ||
		errors.Is(err, lotterydomain.ErrStrategySnapshotSchemaUnsupported) {
		class = marketingapplication.ErrLotteryPublicationInvalid
	}
	return marketingapplication.WrapLotteryVerificationError(class, err)
}

func dependencyIsNil(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice,
		reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
