package core

// merge.go - the shared vocabulary of reconciling disk against a fetch: a new
// day is routine, a changed day is a restatement and must not be appended over.

import (
	"slices"
	"time"
)

// Restatement is one date whose value changed between disk and the fetch. Old
// and New are the canonical renderings, so a report says what the diff would.
type Restatement struct {
	Date     time.Time
	Old, New string
}

// MergeByKey is the shared body of the union merges: sort both sides, index the
// stored one, let the fetched one win. Sorted first, or duplicates decide the bytes.
func MergeByKey[T any](
	stored, fetched []T,
	compare func(a, b T) int,
	key func(T) string,
	dateOf func(T) time.Time,
	value func(T) string,
) ([]T, []Restatement) {
	a, b := slices.Clone(stored), slices.Clone(fetched)
	slices.SortFunc(a, compare)
	slices.SortFunc(b, compare)

	out := make([]T, 0, len(a)+len(b))
	at := map[string]int{}
	for _, p := range a {
		if i, ok := at[key(p)]; ok {
			out[i] = p // a duplicate key within the stored file: the later one stands
			continue
		}
		at[key(p)] = len(out)
		out = append(out, p)
	}

	var rs []Restatement
	for _, p := range b {
		i, ok := at[key(p)]
		if !ok {
			at[key(p)] = len(out)
			out = append(out, p)
			continue
		}
		if old, cur := value(out[i]), value(p); old != cur {
			rs = append(rs, Restatement{Date: dateOf(p), Old: old, New: cur})
		}
		out[i] = p
	}

	slices.SortFunc(out, compare)
	slices.SortFunc(rs, func(x, y Restatement) int { return x.Date.Compare(y.Date) })
	return out, rs
}

// DateSpan is the first and last UTC day a series covers, scanned rather than indexed,
// both zero for an empty one: store.CandleSpan and the etf/price last-date readers.
func DateSpan[T any](in []T, dateOf func(T) time.Time) (first, last time.Time) {
	for _, p := range in {
		d := UTCDay(dateOf(p))
		if first.IsZero() || d.Before(first) {
			first = d
		}
		if d.After(last) {
			last = d
		}
	}
	return first, last
}

// DaysBetween counts whole calendar days between two dates in UTC, either way.
// Each end is reduced to its UTC day, so the answer is a property of the data.
func DaysBetween(from, to time.Time) int {
	f, t := UTCDay(from), UTCDay(to)
	d := int(t.Sub(f) / (24 * time.Hour))
	if d < 0 {
		return -d
	}
	return d
}

// UTCDay reduces an instant to its UTC calendar date at midnight.
func UTCDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
