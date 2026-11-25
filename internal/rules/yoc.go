package rules

// yoc.go - the 10-year yield-on-cost path: each line has a net yield y, a dividend growth g and a price growth p; every year each currency's
// net income is reinvested into the lines that trade in that currency (reinvest.go),
// or pooled into the fund lines when no currency is stated (no DRIP); yield on cost is the
// next year's net income over the money it is measured on. When p = g for every payer and the
// reinvestment basket yields what the whole mix yields, it reduces to Y·(1+Y+G)^n. The Portfolio
// page runs the same steps in JavaScript (simulate in the report template); keep the two in step.

import (
	"math"
	"strconv"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// YoCLine is one line of the projection: the target weight (any scale), the gross yield, the
// withholding, the dividend and price growth (all percent), and whether reinvestment buys it.
// Currency is the currency the dividend is paid in and BuyCurrency the one the line is bought
// with; Acc marks an accumulating fund. With no currencies stated the old pooled rule applies.
type YoCLine struct {
	Weight, YieldPct, WithholdingPct, GrowthPct, PriceGrowthPct float64
	Fund                                                        bool
	Acc                                                         bool
	Currency, BuyCurrency                                       string
}

// YoCYear is one year of the path. Income is what was paid during the year; Forward is what the
// holdings at the year end pay over the next twelve months (both net of withholding).
type YoCYear struct {
	N           int     // years completed, 1..years
	Contributed float64 // all money paid in to date, the start amount included
	Value       float64 // at the year end, in that year's money
	RealValue   float64 // Value / (1+inflation)^N
	Income      float64 // net, paid during the year and reinvested at its end
	Forward     float64 // net, the next twelve months at the year-end holdings
}

// ProjectYoC runs the annual path. The start amount is invested at the target weights on day one;
// the year's monthly contributions (12×monthly) are bought mid-year and earn half that year's
// dividend; at each year end prices step up by p, each currency's net income buys its targets
// (ReinvestTargets: same-currency fund lines, a stock-reinvest currency's income into its stocks, else
// converted at FXCostPct), and the dividend steps up by g.
func ProjectYoC(lines []YoCLine, start, monthly float64, years int, inflationPct float64) []YoCYear {
	s := newYoCSim(lines, start, monthly)
	if s == nil || years <= 0 {
		return nil
	}
	out := make([]YoCYear, 0, years)
	for k := 1; k <= years; k++ {
		income := s.step()
		value := s.value()
		out = append(out, YoCYear{N: k, Contributed: s.contributed, Value: value,
			RealValue: value / math.Pow(1+inflationPct/100, float64(k)), Income: income, Forward: s.forward()})
	}
	return out
}

// yocSim is ProjectYoC's state, stepped one year at a time so the switch to income (switch.go) can
// act on the holdings at a year end. units[i] is what line i holds in start-year money (a unit costs
// 1 at the start and price(i, t) after t years).
type yocSim struct {
	lines       []YoCLine
	w           []float64
	targets     map[string][]Target
	units       []float64
	contributed float64
	c           float64 // 12 × monthly
	k           int     // years completed
}

func newYoCSim(lines []YoCLine, start, monthly float64) *yocSim {
	var total float64
	for _, l := range lines {
		if l.Weight > 0 {
			total += l.Weight
		}
	}
	if total <= 0 {
		return nil
	}
	s := &yocSim{lines: lines, w: make([]float64, len(lines)), targets: map[string][]Target{},
		units: make([]float64, len(lines)), contributed: start, c: 12 * monthly}
	for i, l := range lines {
		if l.Weight > 0 {
			s.w[i] = l.Weight / total
		}
		s.units[i] = start * s.w[i]
	}
	for _, l := range lines {
		c := strings.ToUpper(l.Currency)
		if _, ok := s.targets[c]; !ok {
			s.targets[c] = ReinvestTargets(lines, c)
		}
	}
	return s
}

func (s *yocSim) price(i int, t float64) float64 {
	return math.Pow(1+s.lines[i].PriceGrowthPct/100, t)
}

// netDiv is line i's net dividend per unit over year k+1 (k years of growth).
func (s *yocSim) netDiv(i, k int) float64 {
	l := s.lines[i]
	return l.YieldPct / 100 * (1 - l.WithholdingPct/100) * math.Pow(1+l.GrowthPct/100, float64(k))
}

// step runs one year and returns the net income paid during it, reinvested at its end.
func (s *yocSim) step() float64 {
	s.k++
	k := s.k
	var income float64
	byCcy := map[string]float64{}
	for i := range s.lines {
		bought := 0.0
		if s.c > 0 && s.w[i] > 0 {
			bought = s.c * s.w[i] / s.price(i, float64(k)-0.5)
		}
		inc := (s.units[i] + bought/2) * s.netDiv(i, k-1)
		income += inc
		byCcy[strings.ToUpper(s.lines[i].Currency)] += inc
		s.units[i] += bought
	}
	s.contributed += s.c
	for ccy, inc := range byCcy {
		if inc <= 0 {
			continue
		}
		for _, t := range s.targets[ccy] {
			s.units[t.Line] += inc * t.Share * (1 - t.FXCostPct/100) / s.price(t.Line, float64(k))
		}
	}
	return income
}

// value is the holdings at the current year end, in that year's money.
func (s *yocSim) value() float64 {
	var v float64
	for i := range s.lines {
		v += s.units[i] * s.price(i, float64(s.k))
	}
	return v
}

// forward is the next twelve months' net income at the current holdings.
func (s *yocSim) forward() float64 { return s.forwardAt(0) }

// forwardAt is the net income over the twelve months starting t years after the current year end,
// with no buying and no reinvestment in between.
func (s *yocSim) forwardAt(t int) float64 {
	var f float64
	for i := range s.lines {
		f += s.units[i] * s.netDiv(i, s.k+t)
	}
	return f
}

// PlainYoCPct is the yield on cost with no reinvestment: Σ w·y_net·(1+g)^n over normalised weights.
func PlainYoCPct(lines []YoCLine, n int) float64 {
	var total, s float64
	for _, l := range lines {
		if l.Weight > 0 {
			total += l.Weight
		}
	}
	if total <= 0 {
		return 0
	}
	for _, l := range lines {
		if l.Weight > 0 {
			s += l.Weight / total * l.YieldPct * (1 - l.WithholdingPct/100) * math.Pow(1+l.GrowthPct/100, float64(n))
		}
	}
	return s
}

// ProjectedDividendGrowth is the dividend growth a line is projected on when the allocation states
// none: the strategy's projection_growth_haircut (unset: 1) × the lowest of the 10-, 5- and 3-year
// CAGRs it has (a negative rate is not shrunk), capped at projection_growth_cap_pct (unset: no
// cap). A CAGR measured off a cut or a recovery (59%, say) cannot run for a decade, which is what
// the cap and the haircut are for. basis says which rates were used, the haircut and the cap.
func ProjectedDividendGrowth(g10, g5, g3 *float64) (g *float64, basis string) {
	var parts []string
	m := math.Inf(1)
	for _, c := range []struct {
		name string
		v    *float64
	}{{"10y", g10}, {"5y", g5}, {"3y", g3}} {
		if c.v != nil && !math.IsNaN(*c.v) {
			parts = append(parts, c.name+" "+fmtPct(*c.v))
			m = math.Min(m, *c.v)
		}
	}
	if len(parts) == 0 {
		return nil, ""
	}
	st := pack.Active().Strategy
	hair := 1.0
	if st.ProjectionGrowthHaircut != nil {
		hair = *st.ProjectionGrowthHaircut
	}
	x := m
	if m > 0 {
		x = math.Round(hair*m*100) / 100
	}
	switch {
	case m <= 0:
		basis = "the lowest dividend CAGR (" + strings.Join(parts, ", ") + ") = " + fmtPct(x) + ", not shrunk"
	case st.ProjectionGrowthHaircut == nil:
		basis = "the lowest dividend CAGR (" + strings.Join(parts, ", ") + ") = " + fmtPct(x) + " (no haircut: projection_growth_haircut is not set)"
	default:
		basis = trimFloat(hair) + " × the lowest dividend CAGR (" + strings.Join(parts, ", ") + ") = " + fmtPct(x)
	}
	switch {
	case st.ProjectionGrowthCapPct == nil:
		if x > 0 {
			basis += ", uncapped (projection_growth_cap_pct is not set)"
		}
	case x > *st.ProjectionGrowthCapPct:
		basis += ", capped at " + fmtPct(*st.ProjectionGrowthCapPct) + " for the projection"
		x = *st.ProjectionGrowthCapPct
	}
	return &x, basis
}

func fmtPct(v float64) string { return trimFloat(v) + "%" }

func trimFloat(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }
