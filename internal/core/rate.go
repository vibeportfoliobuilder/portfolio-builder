package core

// rate.go - the three quotients both trees publish, with one rule for no answer: NaN,
// never an infinity and never zero. A caller turns NaN into its own absence once.

import "math"

// Ratio is a / b, NaN where either is no answer, b is zero or the quotient overflows:
// a share of nothing is unanswerable, not infinite.
func Ratio(a, b float64) float64 {
	if !Stated(a) || !Stated(b) || b == 0 {
		return math.NaN()
	}
	return answered(a / b)
}

// GrowthOf is the change from was to now as a fraction of |was|, so a swing out of a
// loss reads positive; NaN where either is no answer or was is zero.
func GrowthOf(now, was float64) float64 {
	if !Stated(now) || !Stated(was) || was == 0 {
		return math.NaN()
	}
	return answered((now - was) / math.Abs(was))
}

// CAGR is the compound annual rate, as a fraction, that turns begin into end over
// years; NaN unless all three are positive answers: no real rate crosses a sign change.
func CAGR(begin, end, years float64) float64 {
	if !Stated(begin) || !Stated(end) || !Stated(years) || begin <= 0 || end <= 0 || years <= 0 {
		return math.NaN()
	}
	return answered(math.Pow(end/begin, 1/years) - 1)
}

// answered is v, or NaN where the arithmetic overflowed into an infinity.
func answered(v float64) float64 {
	if !Stated(v) {
		return math.NaN()
	}
	return v
}
