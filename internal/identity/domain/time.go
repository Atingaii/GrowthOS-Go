package domain

import (
	"fmt"
	"time"
)

func validateCanonicalTime(label string, value time.Time, root error) error {
	if value.IsZero() {
		return fmt.Errorf("%w: %s is required", root, label)
	}
	canonical := value.Round(0).UTC().Truncate(time.Microsecond)
	if value != canonical {
		return fmt.Errorf("%w: %s must be UTC microsecond without monotonic data", root, label)
	}
	return nil
}
