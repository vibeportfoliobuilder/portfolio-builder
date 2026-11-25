package core

// round.go - trimming a computed figure to the places the file it lands in
// carries, in the kernel because three packages round and none may import another.

import "math"

// Stated reports whether v is an answer. A NaN or an infinity is what the
// arithmetic produced when it had none, and no file or page may carry one.
func Stated(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// RoundTo rounds v to n decimal places. NaN and infinity pass through, and so does a
// value too large to scale by 10^n, which has no fraction left to round.
func RoundTo(v float64, n int) float64 {
	if !Stated(v) {
		return v
	}
	p := math.Pow(10, float64(n))
	if r := math.Round(v*p) / p; Stated(r) {
		return r
	}
	return v
}

// Round2 is RoundTo at the two places money, percents and multiples are written
// with.
func Round2(v float64) float64 { return RoundTo(v, 2) }

// StatedPtr is v as an optional value, nil where v is no answer: the one bridge
// from the NaN convention to a model's or a JSON document's absent field.
func StatedPtr(v float64) *float64 {
	if !Stated(v) {
		return nil
	}
	return &v
}

// RoundedPtr is RoundTo(v, n) as an optional value, nil where v is no answer, with
// a negative zero written as 0: a JSON document would print it "-0".
func RoundedPtr(v float64, n int) *float64 {
	r := RoundTo(v, n)
	if r == 0 {
		r = 0
	}
	return StatedPtr(r)
}

// StatedOrZero is v, or zero where v is no answer: the plain form of a figure a
// model spells absent as zero, because no file may carry a NaN.
func StatedOrZero(v float64) float64 {
	if !Stated(v) {
		return 0
	}
	return v
}

// OrNaN is StatedPtr's inverse: an absent optional figure reads as NaN, so it
// goes through the same arithmetic as a present one.
func OrNaN(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

// PctOfFraction turns a published fraction into a percent, 0.65 into 65. It has
// a name because missing the unit change is invisible until a figure is 100x off.
func PctOfFraction(v float64) float64 { return v * 100 }
