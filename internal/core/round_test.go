package core

// round_test.go - the contract the three copies this replaced disagreed about in
// their comments but not in their output: an unusable number stays unusable.

import (
	"math"
	"testing"
)

// TestRoundToReturnsAbsenceAsAbsence: NaN and infinity survive rounding, so a
// figure that was never computed cannot reach a file as 0.00.
func TestRoundToReturnsAbsenceAsAbsence(t *testing.T) {
	if got := Round2(math.NaN()); !math.IsNaN(got) {
		t.Errorf("Round2(NaN) = %v, want NaN", got)
	}
	for _, v := range []float64{math.Inf(1), math.Inf(-1)} {
		if got := Round2(v); got != v {
			t.Errorf("Round2(%v) = %v, want it unchanged", v, got)
		}
	}
	for _, c := range []struct {
		v, want float64
		n       int
	}{
		{1.2349, 1.23, 2},
		{1.2350, 1.24, 2},
		{-1.2350, -1.24, 2},
		{0.00004, 0, 2},
		{0.00004, 0.0000, 4},
		{0.00006, 0.0001, 4},
	} {
		if got := RoundTo(c.v, c.n); got != c.want {
			t.Errorf("RoundTo(%v, %d) = %v, want %v", c.v, c.n, got, c.want)
		}
	}
}
