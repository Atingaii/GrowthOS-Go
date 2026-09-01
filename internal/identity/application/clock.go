package application

import "time"

// Clock supplies one server-owned instant per application operation.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function to Clock.
type ClockFunc func() time.Time

// Now returns the function result.
func (function ClockFunc) Now() time.Time { return function() }

func canonicalInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0).Truncate(time.Microsecond)
}
