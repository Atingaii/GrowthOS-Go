package passwordhash

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClassifiedErrorExposesOnlyStableClasses(t *testing.T) {
	t.Parallel()

	canceled := hashingUnavailable(context.Canceled)
	if !errors.Is(canceled, ErrHashingUnavailable) || !errors.Is(canceled, context.Canceled) {
		t.Fatalf("canceled classification = %v", canceled)
	}
	if errors.Unwrap(canceled) != nil {
		t.Fatalf("classified error exposed an ordinary unwrap chain")
	}
	if canceled.Error() != ErrHashingUnavailable.Error() {
		t.Fatalf("classified error text = %q", canceled.Error())
	}

	sensitive := errors.New("dependency-secret")
	classified := hashingUnavailable(sensitive)
	if !errors.Is(classified, ErrHashingUnavailable) || errors.Is(classified, sensitive) {
		t.Fatalf("dependency classification exposed private cause")
	}
	if strings.Contains(classified.Error(), sensitive.Error()) {
		t.Fatalf("classified error text exposed private cause")
	}
}
