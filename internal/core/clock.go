package core

// clock.go - the one fallback to the wall clock, for an instant a caller may
// inject. Everything below a run takes its instant from here or from its caller.

import "time"

// NowOr is f's instant, or the wall clock where f is nil: what every injectable
// Now func field falls back to.
func NowOr(f func() time.Time) time.Time {
	if f != nil {
		return f()
	}
	return time.Now()
}

// AtOr is t, or the wall clock where t is zero: NowOr for a field that holds the
// instant itself.
func AtOr(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}
