package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

func TestStrategySnapshotRepositoryPortsStayExactAndNarrow(t *testing.T) {
	t.Parallel()

	creatorType := reflect.TypeOf((*StrategySnapshotCreator)(nil)).Elem()
	if creatorType.NumMethod() != 1 {
		t.Fatalf("StrategySnapshotCreator methods = %d, want 1", creatorType.NumMethod())
	}
	create, exists := creatorType.MethodByName("CreateSnapshot")
	if !exists || create.Type.NumIn() != 2 || create.Type.NumOut() != 1 {
		t.Fatalf("CreateSnapshot signature = %v, want context/snapshot -> error", create.Type)
	}
	if got, want := create.Type.In(0), reflect.TypeOf((*context.Context)(nil)).Elem(); got != want {
		t.Fatalf("CreateSnapshot context = %v, want %v", got, want)
	}
	if got, want := create.Type.In(1), reflect.TypeOf(domain.StrategySnapshot{}); got != want {
		t.Fatalf("CreateSnapshot input = %v, want %v", got, want)
	}
	if got, want := create.Type.Out(0), reflect.TypeOf((*error)(nil)).Elem(); got != want {
		t.Fatalf("CreateSnapshot output = %v, want %v", got, want)
	}

	readerType := reflect.TypeOf((*StrategySnapshotReader)(nil)).Elem()
	if readerType.NumMethod() != 1 {
		t.Fatalf("StrategySnapshotReader methods = %d, want 1", readerType.NumMethod())
	}
	find, exists := readerType.MethodByName("FindSnapshotByIdentity")
	if !exists || find.Type.NumIn() != 2 || find.Type.NumOut() != 2 {
		t.Fatalf("FindSnapshotByIdentity signature = %v, want context/identity -> snapshot/error", find.Type)
	}
	if got, want := find.Type.In(0), reflect.TypeOf((*context.Context)(nil)).Elem(); got != want {
		t.Fatalf("FindSnapshotByIdentity context = %v, want %v", got, want)
	}
	if got, want := find.Type.In(1), reflect.TypeOf(domain.StrategySnapshotIdentity{}); got != want {
		t.Fatalf("FindSnapshotByIdentity input = %v, want %v", got, want)
	}
	if got, want := find.Type.Out(0), reflect.TypeOf(domain.StrategySnapshot{}); got != want {
		t.Fatalf("FindSnapshotByIdentity aggregate = %v, want %v", got, want)
	}
	if got, want := find.Type.Out(1), reflect.TypeOf((*error)(nil)).Elem(); got != want {
		t.Fatalf("FindSnapshotByIdentity error = %v, want %v", got, want)
	}
}
