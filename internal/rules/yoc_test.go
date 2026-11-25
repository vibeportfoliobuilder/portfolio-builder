package rules

// yoc_test.go - the yield-on-cost path by the documented method (yoc.go).

import (
	"math"
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack/packtest"
)

func approx(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.6f, want %.6f (±%g)", what, got, want, tol)
	}
}

// Two lines, by hand: stock A 50% (y 6, g 3, p 4) and fund B 50% (y 2, g 5, p 6), 100 in, no
// withholding. Year 1 pays 3 + 1 = 4, which buys B at 1.06: 3.773585 units. Next year A pays
// 50·0.06·1.03 = 3.09 and B 53.773585·0.02·1.05 = 1.129245: 4.219245. Year 2 pays that, which
// buys B at 1.1236: 3.755114 more. Then A pays 50·0.06·1.0609 = 3.1827 and B
// 57.528699·0.02·1.1025 = 1.268508: 4.451208.
func TestYoCTwoHoldingsByHand(t *testing.T) {
	lines := []YoCLine{
		{Weight: 50, YieldPct: 6, GrowthPct: 3, PriceGrowthPct: 4},
		{Weight: 50, YieldPct: 2, GrowthPct: 5, PriceGrowthPct: 6, Fund: true},
	}
	p := ProjectYoC(lines, 100, 0, 2, 3.2)
	if len(p) != 2 {
		t.Fatalf("rows %d", len(p))
	}
	approx(t, "year 1 income", p[0].Income, 4, 1e-9)
	approx(t, "year 1 forward", p[0].Forward, 4.219245, 1e-5)
	approx(t, "year 1 value", p[0].Value, 50*1.04+53.773585*1.06, 1e-5)
	approx(t, "year 1 real value", p[0].RealValue, p[0].Value/1.032, 1e-9)
	approx(t, "year 2 income", p[1].Income, 4.219245, 1e-5)
	approx(t, "year 2 forward", p[1].Forward, 4.451208, 1e-5)
	approx(t, "plain YoC year 2", PlainYoCPct(lines, 2), 0.5*6*1.0609+0.5*2*1.1025, 1e-9)
}

// With p = g for every payer and the reinvestment basket yielding what the whole mix yields, the
// path is Y·(1+Y+G)^n: Y = Σ w·y (net), G = Σ w·p.
func TestYoCReducesToYTimesOnePlusRToTheN(t *testing.T) {
	lines := []YoCLine{
		{Weight: 30, YieldPct: 5, WithholdingPct: 0, GrowthPct: 5, PriceGrowthPct: 5, Fund: true},
		{Weight: 70, YieldPct: 5, WithholdingPct: 0, GrowthPct: 5, PriceGrowthPct: 5, Fund: true},
	}
	Y, G := 0.05, 0.05
	p := ProjectYoC(lines, 1000, 0, 10, 3.2)
	for _, y := range p {
		approx(t, "YoC", y.Forward/1000*100, Y*math.Pow(1+Y+G, float64(y.N))*100, 1e-9)
	}
	// Different yields across funds, the same p = g: units stay in target proportion, so it still reduces.
	lines = []YoCLine{
		{Weight: 40, YieldPct: 8, WithholdingPct: 25, GrowthPct: 4, PriceGrowthPct: 4, Fund: true},
		{Weight: 60, YieldPct: 2, GrowthPct: 4, PriceGrowthPct: 4, Fund: true},
	}
	Y, G = 0.4*0.06+0.6*0.02, 0.04
	p = ProjectYoC(lines, 1000, 0, 10, 3.2)
	approx(t, "YoC year 10", p[9].Forward/1000*100, Y*math.Pow(1+Y+G, 10)*100, 1e-9)
	approx(t, "value year 10", p[9].Value, 1000*math.Pow(1+Y+G, 10), 1e-6)
}

// The yield on the start money counts only the start money: adding monthly contributions changes
// the all-money path, never the start-money one.
func TestMonthlyContributionsDoNotInflateYoCOnTheStartMoney(t *testing.T) {
	a := sample()
	r0 := CheckPortfolio(func() Allocation { b := sample(); b.Monthly = 0; return b }(), homeFX)
	r1 := CheckPortfolio(a, homeFX)
	for i := range r1.Path {
		if r0.Path[i].CohortYoCPct != r1.Path[i].CohortYoCPct {
			t.Errorf("year %d: start-money YoC %v with contributions, %v without", r1.Path[i].Year, r1.Path[i].CohortYoCPct, r0.Path[i].CohortYoCPct)
		}
	}
	last := r1.Path[9]
	// The sample (start yield ~5.7%, growth 0–12%) lands in the realistic band, not at 70%.
	if last.CohortYoCPct < 8 || last.CohortYoCPct > 16 {
		t.Errorf("year-10 YoC on the start money %v, want 8–16", last.CohortYoCPct)
	}
	if last.Contributed != 100_000+20_000*12*10 {
		t.Errorf("contributed %v", last.Contributed)
	}
	if last.RealValue >= last.Value || math.Abs(last.RealValue-last.Value/math.Pow(1.032, 10)) > 1 {
		t.Errorf("real %v nominal %v", last.RealValue, last.Value)
	}
}

// The test strategy's projection_growth_haircut is 0.75 and projection_growth_cap_pct 9.
func TestProjectedDividendGrowthIsCapped(t *testing.T) {
	g, basis := ProjectedDividendGrowth(nil, p(59.33), p(71.37))
	if g == nil || *g != 9 || basis != "0.75 × the lowest dividend CAGR (5y 59.33%, 3y 71.37%) = 44.5%, capped at 9% for the projection" {
		t.Errorf("%v %q", g, basis)
	}
	g, basis = ProjectedDividendGrowth(nil, nil, p(4))
	if g == nil || *g != 3 || basis != "0.75 × the lowest dividend CAGR (3y 4%) = 3%" {
		t.Errorf("%v %q", g, basis)
	}
	// The 10-year rate is the lowest.
	g, _ = ProjectedDividendGrowth(p(4.66), nil, p(38.23))
	if g == nil || *g != 3.5 {
		t.Errorf("lowest 10y %v", g)
	}
	// A negative rate is not shrunk toward zero.
	if g, basis := ProjectedDividendGrowth(p(2), p(-5), p(1)); g == nil || *g != -5 || !strings.Contains(basis, "not shrunk") {
		t.Errorf("negative %v %q", g, basis)
	}
	if g, _ := ProjectedDividendGrowth(nil, nil, nil); g != nil {
		t.Errorf("%v", g)
	}
}

// With no haircut or cap in the strategy the lowest CAGR is projected as it is, and the basis says so.
func TestProjectedDividendGrowthWithNoCapSet(t *testing.T) {
	s := packtest.Set()
	s.Strategy.ProjectionGrowthCapPct, s.Strategy.ProjectionGrowthHaircut = nil, nil
	pack.Use(s)
	t.Cleanup(packtest.Activate)
	g, basis := ProjectedDividendGrowth(nil, p(12), p(15))
	if g == nil || *g != 12 || !strings.Contains(basis, "no haircut") || !strings.Contains(basis, "uncapped") {
		t.Errorf("%v %q", g, basis)
	}
}
