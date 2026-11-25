package divscore

import (
	"fmt"
	"maps"
	"slices"
	"sort"
)

// The absolute/relative weights of the combined figure.
const (
	SafetyWeight    = 0.60
	CompositeWeight = 0.40
)

// MinRanked is how many companies must state a factor before a place among them means anything.
const MinRanked = 3

// FactorNames are the four factors of the composite, in the order the page reads them.
var FactorNames = []string{"cash flow to debt", "return on equity", "yield", "dividend growth"}

func factorOf(f Factors, name string) *float64 {
	switch name {
	case "cash flow to debt":
		return f.CashFlowToDebt
	case "return on equity":
		return f.ReturnOnEquity
	case "yield":
		return f.Yield
	case "dividend growth":
		return f.DividendGrowth5
	}
	return nil
}

// Member is one company of the set being ranked: its stored score and its yield.
type Member struct {
	Score Score
	Yield float64
}

// Rank is one company's place in the set.
type Rank struct {
	Safety      *float64           `json:"safety,omitempty"`
	Composite   *float64           `json:"composite,omitempty"`
	Combined    *float64           `json:"combined,omitempty"`
	Band        string             `json:"band,omitempty"`
	Gate        bool               `json:"gate"`
	GateWhy     string             `json:"gate_why,omitempty"`
	Percentiles map[string]float64 `json:"percentiles,omitempty"`
	CappedBy    string             `json:"capped_by,omitempty"`
}

// Set is one whole read.
type Set struct {
	Ranks        map[string]Rank `json:"ranks,omitempty"`
	Companies    int             `json:"companies"`
	AverageYield float64         `json:"average_yield,omitempty"`
	Note         string          `json:"note,omitempty"`
}

// NoteRelative is what every reader of a percentile has to be told.
const NoteRelative = "the composite, the combined figure and the yield half of the screen are places among the %d companies read here and nothing else: read a different set and they move, while the safety figure does not"

// RankSet strikes the relative half over the companies given. Nothing is stored.
func RankSet(ms map[string]Member) Set {
	out := Set{Ranks: map[string]Rank{}, Companies: len(ms)}
	if len(ms) == 0 {
		return out
	}
	tickers := slices.Sorted(maps.Keys(ms))
	places := map[string]map[string]float64{}
	for _, name := range FactorNames {
		vals := map[string]float64{}
		for _, t := range tickers {
			if v := factorOf(ms[t].Score.Factors, name); v != nil {
				vals[t] = *v
			}
		}
		for t, p := range percentiles(vals) {
			if places[t] == nil {
				places[t] = map[string]float64{}
			}
			places[t][name] = p
		}
	}
	var sum float64
	var n int
	for _, t := range tickers {
		if y := ms[t].Yield; y > 0 {
			sum, n = sum+y, n+1
		}
	}
	if n > 0 {
		out.AverageYield = round2(sum / float64(n))
	}
	for _, t := range tickers {
		out.Ranks[t] = rankOne(ms[t], places[t], out.AverageYield)
	}
	out.Note = fmt.Sprintf(NoteRelative, out.Companies)
	return out
}

func rankOne(m Member, places map[string]float64, avgYield float64) Rank {
	s := m.Score
	r := Rank{Safety: s.Safety, Band: s.Band, Percentiles: places}
	r.Composite = compositeOf(places)
	if s.Safety != nil && r.Composite != nil {
		v, by := Capped(SafetyWeight**s.Safety+CompositeWeight**r.Composite, s.Caps)
		v = round2(v)
		r.Combined, r.Band, r.CappedBy = &v, Band(v), by
	}
	switch y := m.Yield; {
	case !s.TenYearRecord:
		r.GateWhy = "fewer than ten years of payments on record"
	case avgYield > 0 && y <= avgYield:
		r.GateWhy = fmt.Sprintf("a yield of %.2f%% against this set's average of %.2f%%", y, avgYield)
	default:
		r.Gate = true
	}
	return r
}

// compositeOf averages the places over every factor, an unranked one counting as the middle.
func compositeOf(places map[string]float64) *float64 {
	if len(places) == 0 {
		return nil
	}
	var sum float64
	for _, name := range FactorNames {
		if p, ok := places[name]; ok {
			sum += p
		} else {
			sum += 50
		}
	}
	c := round2(sum / float64(len(FactorNames)))
	return &c
}

// percentiles places each value as the share of the set it is at least as large as; a set
// smaller than MinRanked is not ranked at all.
func percentiles(vals map[string]float64) map[string]float64 {
	if len(vals) < MinRanked {
		return nil
	}
	sorted := slices.Sorted(maps.Values(vals))
	out := make(map[string]float64, len(vals))
	for t, v := range vals {
		n := sort.SearchFloat64s(sorted, v)
		for n < len(sorted) && sorted[n] <= v {
			n++
		}
		out[t] = round2(float64(n) / float64(len(sorted)) * 100)
	}
	return out
}
