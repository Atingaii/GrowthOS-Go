package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

func TestStrategyRoutingGraphRepositoryPortsStayAggregateScopedAndNarrow(t *testing.T) {
	t.Parallel()

	creatorType := reflect.TypeOf((*StrategyRoutingGraphCreator)(nil)).Elem()
	if creatorType.NumMethod() != 1 {
		t.Fatalf("StrategyRoutingGraphCreator methods = %d, want 1", creatorType.NumMethod())
	}
	create, exists := creatorType.MethodByName("Create")
	if !exists {
		t.Fatal("StrategyRoutingGraphCreator does not expose Create")
	}
	if got, want := create.Type.NumIn(), 2; got != want {
		t.Fatalf("Create inputs = %d, want %d", got, want)
	}
	if got, want := create.Type.In(0), reflect.TypeOf((*context.Context)(nil)).Elem(); got != want {
		t.Fatalf("Create context input = %v, want %v", got, want)
	}
	if got, want := create.Type.In(1), reflect.TypeOf(domain.StrategyRoutingGraph{}); got != want {
		t.Fatalf("Create aggregate input = %v, want %v", got, want)
	}
	if got, want := create.Type.NumOut(), 1; got != want || create.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("Create outputs = %v, want one error", create.Type)
	}

	readerType := reflect.TypeOf((*StrategyRoutingGraphReader)(nil)).Elem()
	if readerType.NumMethod() != 1 {
		t.Fatalf("StrategyRoutingGraphReader methods = %d, want 1", readerType.NumMethod())
	}
	find, exists := readerType.MethodByName("FindByIdentity")
	if !exists {
		t.Fatal("StrategyRoutingGraphReader does not expose FindByIdentity")
	}
	if got, want := find.Type.NumIn(), 2; got != want {
		t.Fatalf("FindByIdentity inputs = %d, want %d", got, want)
	}
	if got, want := find.Type.In(0), reflect.TypeOf((*context.Context)(nil)).Elem(); got != want {
		t.Fatalf("FindByIdentity context input = %v, want %v", got, want)
	}
	if got, want := find.Type.In(1), reflect.TypeOf(domain.StrategyRoutingGraphIdentity{}); got != want {
		t.Fatalf("FindByIdentity lookup input = %v, want %v", got, want)
	}
	if got, want := find.Type.NumOut(), 2; got != want {
		t.Fatalf("FindByIdentity outputs = %d, want %d", got, want)
	}
	if got, want := find.Type.Out(0), reflect.TypeOf(domain.StrategyRoutingGraph{}); got != want {
		t.Fatalf("FindByIdentity aggregate output = %v, want %v", got, want)
	}
	if got, want := find.Type.Out(1), reflect.TypeOf((*error)(nil)).Elem(); got != want {
		t.Fatalf("FindByIdentity error output = %v, want %v", got, want)
	}
}
