package rules

// switch_test.go - the switch to income (switch.go), by hand and by its rules.

import (
	"math"
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack/packtest"
)

func switchAlloc() Allocation {
	zero := 0.0
	return Allocation{Currency: "XTS", StartAmount: 1000, Monthly: 0, Years: 1, StartYear: 2031,
		Switch: &SwitchParams{Years: []int{1}, BondYieldsPct: []float64{5}, BondCapPct: p(10), SellPct: p(60), StockSharePct: p(50), HigherMonthly: 1},
		Lines: []Line{
			{Ticker: "A", Kind: KindStock, WeightPct: 40, YieldPct: p(5), DividendGrowthPct: &zero, PriceGrowthPct: &zero, Country: "ZZ", DividendCurrency: "XTS", TradingCurrency: "XTS"},
			{Ticker: "I", Kind: KindETFDist, Bucket: "income", WeightPct: 20, YieldPct: p(10), DividendGrowthPct: &zero, PriceGrowthPct: &zero, DividendCurrency: "USD", TradingCurrency: "USD"},
			{Ticker: "G", Kind: KindETFAcc, Bucket: "all_cap", WeightPct: 40, PriceGrowthPct: &zero, TradingCurrency: "USD"},
		}}
}

// By hand, per 100 of the 1000 start: year 1 pays A 2 XTS (XTS is a stock-reinvest currency: back
// into A, 42) and I 2 USD (into the USD funds I and G by weight: I 20.6667, G 41.3333). Value 104.
// The switch sells 60% of G (24.8 USD); bonds 10% of 104 = 10.4, less 0.4% FX = 10.3584 held; the
// rest 14.4 is half A (USD->XTS, 0.4% FX: 7.1712) and half I (7.2). Income after: 49.1712·5% +
// 27.8667·10% + 10.3584·5% = 5.76315 a year, so 57.6315 on the 1000.
func TestTheSwitchByHand(t *testing.T) {
	r := CheckPortfolio(switchAlloc(), homeFX)
	if r.Switch == nil || len(r.Switch.Rows) == 0 {
		t.Fatalf("no switch: %+v", r.Switch)
	}
	x := r.Switch.Rows[0]
	if x.Monthly != 0 || x.Year != 2032 || x.AfterYears != 1 || x.BondYieldPct != 5 {
		t.Fatalf("row %+v", x)
	}
	approx(t, "value", x.Value, 1040, 0.5)
	approx(t, "sold", x.Sold, 248, 0.5)
	approx(t, "bonds", x.Bonds, 104, 0.5)
	approx(t, "yoc after", x.YoCPct, 5.76315, 0.006)
	approx(t, "start-money yoc", x.StartMoneyYoCPct, 5.76315, 0.006)
	if x.Growth10Pct != 0 || x.GrowthStatus != Fail {
		t.Errorf("no growth anywhere: %v %s", x.Growth10Pct, x.GrowthStatus)
	}
	if got := r.Switch.Sold; len(got) != 1 || got[0] != "G" {
		t.Errorf("sold %v", got)
	}
	if len(r.Switch.StockTargets) != 1 || len(r.Switch.IncomeTargets) != 1 {
		t.Errorf("targets %v %v", r.Switch.StockTargets, r.Switch.IncomeTargets)
	}
}

// The same by hand at scale, so the monthly figures are whole units.
func TestTheSwitchMonthlyIncomeAtScale(t *testing.T) {
	a := switchAlloc()
	a.StartAmount = 12_000_000
	x := CheckPortfolio(a, homeFX).Switch.Rows[0]
	approx(t, "monthly before", x.MonthlyBefore, 4.166667*120000/12, 1)
	approx(t, "monthly after", x.MonthlyIncome, 5.76315*120000/12, 1)
	approx(t, "real", x.MonthlyIncomeReal, 5.76315*120000/12/1.032, 1)
	approx(t, "gap", x.TargetGap, 5.76315*120000/12-DefaultTargetMonthly(), 1)
}

func TestTheSwitchDefaults(t *testing.T) {
	r := CheckPortfolio(sample(), homeFX)
	sw := r.Switch
	if sw == nil {
		t.Fatal("no switch section")
	}
	// 2 monthly figures × 3 switch years × 2 bond yields.
	if len(sw.Rows) != 12 || len(sw.Reach) != 4 {
		t.Fatalf("rows %d reach %d", len(sw.Rows), len(sw.Reach))
	}
	years := map[int]bool{}
	for _, x := range sw.Rows {
		years[x.Year] = true
		if x.Bonds > 0.20*x.Value+1 {
			t.Errorf("bonds over the 20%% cap of the value: %+v", x)
		}
		if x.MonthlyIncome <= x.MonthlyBefore {
			t.Errorf("the switch did not raise the income: %+v", x)
		}
	}
	if !years[2039] || !years[2043] || !years[2047] {
		t.Errorf("switch years %v", years)
	}
	if sw.Rows[0].Monthly != 20_000 || sw.Rows[6].Monthly != DefaultHigherMonthly() {
		t.Errorf("monthly figures %v %v", sw.Rows[0].Monthly, sw.Rows[6].Monthly)
	}
	// TWSC (1.2% yield, under low_yield_pct) is sold, FWLD (no bucket, distributing) kept; FINC has no
	// income bucket in the sample, so it is not a target either.
	if !contains(sw.Sold, "TWSC") || contains(sw.Sold, "FWLD") || contains(sw.IncomeTargets, "FINC") {
		t.Errorf("sold %v income %v", sw.Sold, sw.IncomeTargets)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// The year the target is first reached is the first switch whose monthly income reaches it.
func TestTheTargetYearIsTheFirstSwitchThatReachesIt(t *testing.T) {
	a := sample()
	a.Switch = &SwitchParams{Years: []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, BondYieldsPct: []float64{4.25}}
	sw := CheckPortfolio(a, homeFX).Switch
	target := sw.Rows[5].MonthlyIncome - 1 // reached at year 15 (2046), not before
	a.Switch.TargetMonthly = target
	sw = CheckPortfolio(a, homeFX).Switch
	r := sw.Reach[0]
	if r.Year == nil || *r.Year != 2046 || *r.AfterYears != 15 {
		t.Fatalf("reach %+v (rows %v)", r, sw.Rows[4:6])
	}
	if r.RealYear == nil || *r.RealYear <= 2046 {
		t.Errorf("in today's money the target comes later: %+v", r)
	}
}

// The glide moves part of the sale before the switch and never buys bonds early; with no glide
// the whole share is sold at the switch.
func TestTheGlideNeverBuysBondsEarly(t *testing.T) {
	a := sample()
	a.Lines = append(a.Lines, Line{Ticker: "VALL", Kind: KindETFAcc, Bucket: "all_cap", WeightPct: 20, TradingCurrency: "USD"})
	a.Switch = &SwitchParams{Years: []int{10}, BondYieldsPct: []float64{4.25}}
	base := CheckPortfolio(a, homeFX).Switch.Rows[0]
	a.Switch.GlideYears = 3
	g := CheckPortfolio(a, homeFX).Switch
	x := g.Rows[0]
	if base.Glided != 0 || x.Glided <= 0 || x.Sold >= base.Sold {
		t.Errorf("glide: base %+v glide %+v", base, x)
	}
	if x.Bonds > x.Sold+1 {
		t.Errorf("bonds %v from %v sold at the switch", x.Bonds, x.Sold)
	}
	if len(g.Notes) == 0 {
		t.Error("the glide is not described")
	}
	if math.Abs(x.Value-base.Value) > 0.05*base.Value {
		t.Errorf("the glide changed the value too much: %v vs %v", x.Value, base.Value)
	}
}

func TestSwitchRoles(t *testing.T) {
	for _, tc := range []struct {
		l    Line
		want string
	}{
		{Line{Kind: KindETFAcc, Bucket: "growth"}, RoleSell},
		{Line{Kind: KindETFAcc, Bucket: "all_cap"}, RoleSell},
		{Line{Kind: KindETFAcc}, RoleSell},
		{Line{Kind: KindETFDist, Bucket: "income"}, RoleIncome},
		{Line{Kind: KindETFDist, Bucket: "dividend_growth"}, RoleKeep},
		{Line{Kind: KindETFAcc, Bucket: "all_cap", SwitchRole: "keep"}, RoleKeep},
		{Line{Kind: KindStock, Group: "Growth", YieldPct: p(3)}, RoleSell},
		{Line{Kind: KindStock, Group: "Grower", YieldPct: p(0.8)}, RoleSell},
		{Line{Kind: KindStock, Group: "Grower", YieldPct: p(0.8), SwitchRole: "keep"}, RoleStock},
		{Line{Kind: KindStock, Group: "GOAT", YieldPct: p(7)}, RoleStock},
		{Line{Kind: KindStock, Group: "Yielder", YieldPct: p(7), SwitchRole: "sell"}, RoleSell},
		{Line{Kind: KindStock, YieldPct: p(9), Moonshot: true}, RoleKeep},
		{Line{Kind: KindStock}, RoleSell},
	} {
		if got := SwitchRole(tc.l); got != tc.want {
			t.Errorf("%+v: %q want %q", tc.l, got, tc.want)
		}
	}
}

// With no target and no higher contribution in the strategy the switch rows are still projected at
// the mix's own amount, and no target year is searched (no zero target "reached" in year 1).
func TestTheSwitchWithNoTarget(t *testing.T) {
	s := packtest.Set()
	s.Strategy.TargetMonthly, s.Strategy.HigherMonthly = nil, nil
	pack.Use(s)
	t.Cleanup(packtest.Activate)
	sw := CheckPortfolio(sample(), homeFX).Switch
	if len(sw.Rows) != 6 || len(sw.Reach) != 0 {
		t.Fatalf("rows %d reach %d", len(sw.Rows), len(sw.Reach))
	}
	for _, x := range sw.Rows {
		if x.TargetGap != 0 || x.Monthly != 20_000 {
			t.Errorf("row %+v", x)
		}
	}
	found := false
	for _, n := range sw.Notes {
		found = found || strings.Contains(n, "target_monthly")
	}
	if !found {
		t.Errorf("notes %v", sw.Notes)
	}
}

// With no bond cap in the strategy no switch is projected, and the note names the key.
func TestTheSwitchNeedsABondCap(t *testing.T) {
	s := packtest.Set()
	s.Strategy.BondCapPct = nil
	pack.Use(s)
	t.Cleanup(packtest.Activate)
	sw := CheckPortfolio(sample(), homeFX).Switch
	if len(sw.Rows) != 0 || len(sw.Notes) == 0 || !strings.Contains(sw.Notes[0], "bond_cap_pct") {
		t.Errorf("%+v", sw)
	}
}
