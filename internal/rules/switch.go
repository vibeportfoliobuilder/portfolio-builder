package rules

// switch.go - the switch to income. Until the money is needed the mix runs the yield-on-cost path
// (yoc.go); at the switch the strategy's switch_sell_pct of the growth and all-cap holdings is
// sold, the rest stays, and the dividend payers are kept. The proceeds buy the country pack's
// bonds (in its currency, bought only at the switch) up to the strategy's bond_cap_pct of the
// portfolio value, and the rest is split between the mix's own dividend stocks and its
// covered-call (income bucket) funds (switch_stock_share_pct), at their weights and net yields. After the switch the
// income is lived on: no contributions, no reinvestment. The Portfolio page runs the same steps
// (switchModel in the report template); keep the two in step.

import (
	"fmt"
	"math"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// Switch roles of a line (SwitchRole).
const (
	RoleSell   = "sell"   // growth and all-cap holdings: switch_sell_pct of them sold at the switch
	RoleStock  = "stock"  // dividend stocks: kept, and bought with the proceeds
	RoleIncome = "income" // covered-call (income bucket) funds: kept, and bought with the proceeds
	RoleKeep   = ""       // other payers (dividend-growth funds, moonshots): kept, not bought
)

// The switch's own search limits. The years, the share sold, the stock share, the bond cap, the
// target income and the higher contribution are the investor's strategy; the bond yields the
// country pack's.
const (
	switchSearchMaxYears  = 40
	switchGrowthHorizonYr = 10
	maxGlideYears         = 5
)

// DefaultBondCapPct is the strategy's bond cap (nil when unset).
func DefaultBondCapPct() *float64 { return pack.Active().Strategy.BondCapPct }

// DefaultBondYieldsPct are the country pack's bond yields the switch is shown at.
func DefaultBondYieldsPct() []float64 { return pack.Active().Country.Bonds.YieldsPct }

// DefaultTargetMonthly is the strategy's net monthly income target (0 when unset).
func DefaultTargetMonthly() float64 { return deref(pack.Active().Strategy.TargetMonthly) }

// DefaultHigherMonthly is the strategy's higher monthly contribution (0 when unset).
func DefaultHigherMonthly() float64 { return deref(pack.Active().Strategy.HigherMonthly) }

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// SwitchParams are the switch's inputs; zero values take the defaults. A mix may state them under
// `switch:`; pb portfolio check's flags override the mix.
type SwitchParams struct {
	Years         []int     `yaml:"years,omitempty" json:"years,omitempty"`
	SellPct       *float64  `yaml:"sell_pct,omitempty" json:"sell_pct,omitempty"`
	BondCapPct    *float64  `yaml:"bond_cap_pct,omitempty" json:"bond_cap_pct,omitempty"`
	BondYieldsPct []float64 `yaml:"bond_yields_pct,omitempty" json:"bond_yields_pct,omitempty"`
	// StockSharePct is the share of the proceeds left after the bonds that buys dividend stocks;
	// the rest buys covered-call funds.
	StockSharePct *float64 `yaml:"stock_share_pct,omitempty" json:"stock_share_pct,omitempty"`
	// GlideYears moves part of the sale into dividend stocks and income funds over the last N
	// year ends before the switch (0 = off, at most 5). Bonds are never bought before the switch.
	GlideYears    int     `yaml:"glide_years,omitempty" json:"glide_years,omitempty"`
	TargetMonthly float64 `yaml:"target_monthly,omitempty" json:"target_monthly,omitempty"`
	HigherMonthly float64 `yaml:"higher_monthly,omitempty" json:"higher_monthly,omitempty"`
}

// Resolved fills the defaults.
func (p SwitchParams) Resolved() SwitchParams {
	st := pack.Active().Strategy
	if len(p.Years) == 0 {
		p.Years = st.SwitchYears
	}
	f := func(v *float64, d float64) *float64 {
		if v == nil {
			return &d
		}
		x := math.Max(0, math.Min(100, *v))
		return &x
	}
	if p.SellPct == nil {
		p.SellPct = st.SwitchSellPct
	}
	if p.SellPct != nil {
		p.SellPct = f(p.SellPct, 0)
	}
	if p.BondCapPct == nil {
		p.BondCapPct = DefaultBondCapPct()
	}
	if p.BondCapPct != nil {
		p.BondCapPct = f(p.BondCapPct, 0)
	}
	if p.StockSharePct == nil {
		p.StockSharePct = st.SwitchStockSharePct
	}
	if p.StockSharePct != nil {
		p.StockSharePct = f(p.StockSharePct, 0)
	}
	if len(p.BondYieldsPct) == 0 {
		p.BondYieldsPct = DefaultBondYieldsPct()
	}
	p.GlideYears = max(0, min(maxGlideYears, p.GlideYears))
	if p.TargetMonthly <= 0 {
		p.TargetMonthly = DefaultTargetMonthly()
	}
	if p.HigherMonthly <= 0 {
		p.HigherMonthly = DefaultHigherMonthly()
	}
	return p
}

// SwitchRole is what the switch does with a line. Sold: growth and all-cap funds (an accumulating
// fund with no bucket counts as one), and stocks in the Growth group, in the growth bucket or
// yielding under the strategy's low_yield_pct (a low dividend held for price growth). Bought:
// income-bucket distributing funds and the other dividend stocks. Kept and not bought: moonshots
// and other funds. A line's `switch: sell|keep` overrides the rule.
func SwitchRole(l Line) string {
	b := strings.ToLower(l.Bucket)
	over := strings.ToLower(l.SwitchRole)
	if over == RoleSell {
		return RoleSell
	}
	keep := over == "keep"
	y := lineYield(l)
	if l.IsETF() {
		switch {
		case b == "income" && l.Kind == KindETFDist:
			return RoleIncome
		case keep:
			return RoleKeep
		case b == "growth" || b == "all_cap" || (b == "" && l.Kind == KindETFAcc):
			return RoleSell
		}
		return RoleKeep
	}
	if l.Moonshot {
		return RoleKeep
	}
	low := pack.Active().Strategy.LowYieldPct
	growth := strings.EqualFold(l.Group, Growth) || b == "growth" || y == nil || (low != nil && *y < *low)
	switch {
	case growth && !keep:
		return RoleSell
	case y != nil && *y > 0:
		return RoleStock
	}
	return RoleKeep
}

// SwitchRow is one switch: after AfterYears of the path, at one monthly contribution and one bond
// yield. Year is the first calendar year lived on (start_year + AfterYears). Money is nominal, in the mix currency,
// unless named real; incomes are net of withholding, monthly = the next twelve months / 12.
type SwitchRow struct {
	Monthly           float64 `yaml:"monthly" json:"monthly"`
	AfterYears        int     `yaml:"after_years" json:"after_years"`
	Year              int     `yaml:"year" json:"year"`
	BondYieldPct      float64 `yaml:"bond_yield_pct" json:"bond_yield_pct"`
	Contributed       float64 `yaml:"contributed" json:"contributed"`
	Value             float64 `yaml:"value" json:"value"`
	RealValue         float64 `yaml:"real_value" json:"real_value"`
	Sold              float64 `yaml:"sold" json:"sold"`
	Glided            float64 `yaml:"glided,omitempty" json:"glided,omitempty"`
	Bonds             float64 `yaml:"bonds" json:"bonds"`
	ToStocks          float64 `yaml:"to_dividend_stocks" json:"to_dividend_stocks"`
	ToIncomeFunds     float64 `yaml:"to_income_funds" json:"to_income_funds"`
	FXCost            float64 `yaml:"fx_cost" json:"fx_cost"`
	MonthlyBefore     float64 `yaml:"monthly_income_before" json:"monthly_income_before"`
	MonthlyIncome     float64 `yaml:"monthly_income" json:"monthly_income"`
	MonthlyIncomeReal float64 `yaml:"monthly_income_real" json:"monthly_income_real"`
	YoCPct            float64 `yaml:"yoc_on_contributed_pct" json:"yoc_on_contributed_pct"`
	StartMoneyYoCPct  float64 `yaml:"start_money_yoc_pct" json:"start_money_yoc_pct"`
	Growth10Pct       float64 `yaml:"income_growth_10y_pct" json:"income_growth_10y_pct"`
	GrowthStatus      string  `yaml:"income_growth_status" json:"income_growth_status"`
	TargetGap         float64 `yaml:"target_gap_monthly" json:"target_gap_monthly"`
}

// SwitchReach is the first switch at which the net monthly income reaches the target.
type SwitchReach struct {
	Monthly        float64 `yaml:"monthly" json:"monthly"`
	BondYieldPct   float64 `yaml:"bond_yield_pct" json:"bond_yield_pct"`
	Year           *int    `yaml:"year,omitempty" json:"year,omitempty"`
	AfterYears     *int    `yaml:"after_years,omitempty" json:"after_years,omitempty"`
	RealYear       *int    `yaml:"real_year,omitempty" json:"real_year,omitempty"`
	RealAfterYears *int    `yaml:"real_after_years,omitempty" json:"real_after_years,omitempty"`
	Note           string  `yaml:"note,omitempty" json:"note,omitempty"`
}

// SwitchReport is the switch section of pb portfolio check.
type SwitchReport struct {
	Params        SwitchParams  `yaml:"params" json:"params"`
	Sold          []string      `yaml:"sold" json:"sold"`
	StockTargets  []string      `yaml:"dividend_stock_targets" json:"dividend_stock_targets"`
	IncomeTargets []string      `yaml:"income_fund_targets" json:"income_fund_targets"`
	Rows          []SwitchRow   `yaml:"rows" json:"rows"`
	Reach         []SwitchReach `yaml:"target_reached" json:"target_reached"`
	Notes         []string      `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// switchPlan is the switch applied to one simulation.
type switchPlan struct {
	roles      []string
	sellFrac   float64 // fraction of a sold line's units sold at each step (glide steps and the switch)
	bondCap    float64 // of the portfolio value
	stockShare float64
	glide      int
}

type switchResult struct {
	value, sold, glided, bonds, toStocks, toIncome, fx float64
	before, after, after10                             float64
}

// runSwitch runs the path for k years with the glide before it, applies the switch at the end of
// year k and returns the figures. bondYield is percent.
func runSwitch(lines []YoCLine, pl switchPlan, start, monthly float64, k int, bondYield float64) (switchResult, bool) {
	s := newYoCSim(lines, start, monthly)
	if s == nil || k <= 0 {
		return switchResult{}, false
	}
	var res switchResult
	for j := 1; j <= k; j++ {
		s.step()
		if j < k && j >= k-pl.glide {
			// A glide step: part of the growth holdings moves into the payers; no bonds yet.
			proceeds := pl.sell(s)
			res.glided += total(proceeds)
			res.fx += pl.buy(s, proceeds, total(proceeds), 0)
		}
	}
	res.before = s.forward()
	res.value = s.value()
	proceeds := pl.sell(s)
	res.sold = total(proceeds)
	res.bonds = math.Min(res.sold, pl.bondCap/100*res.value)
	if len(pl.targets(s, RoleStock))+len(pl.targets(s, RoleIncome)) == 0 {
		res.bonds = res.sold // nothing to buy: all the proceeds go to bonds (noted in the report)
	}
	// Bonds are bought in their own currency: proceeds from lines traded in another pay the FX cost.
	var bondFX float64
	bondCcy := strings.ToUpper(pack.Active().Country.Bonds.Currency)
	if res.sold > 0 {
		for c, v := range proceeds {
			if c != "" && c != bondCcy {
				bondFX += v * res.bonds / res.sold * FXCostPct() / 100
			}
		}
	}
	held := res.bonds - bondFX
	res.fx += bondFX
	res.toStocks, res.toIncome = pl.split(s, res.sold-res.bonds)
	res.fx += pl.buy(s, proceeds, res.sold, res.bonds)
	res.after = s.forward() + held*bondYield/100
	res.after10 = s.forwardAt(switchGrowthHorizonYr) + held*bondYield/100
	return res, true
}

func total(m map[string]float64) float64 {
	var t float64
	for _, v := range m {
		t += v
	}
	return t
}

// sell takes sellFrac of every sold line's units and returns the proceeds by the currency each
// line trades in.
func (pl switchPlan) sell(s *yocSim) map[string]float64 {
	out := map[string]float64{}
	for i, l := range s.lines {
		if pl.roles[i] != RoleSell || s.units[i] <= 0 {
			continue
		}
		u := s.units[i] * pl.sellFrac
		s.units[i] -= u
		out[strings.ToUpper(l.BuyCurrency)] += u * s.price(i, float64(s.k))
	}
	return out
}

// targets are the lines of one role the proceeds buy, at their target weights.
func (pl switchPlan) targets(s *yocSim, role string) []Target {
	var tot float64
	for i, l := range s.lines {
		if pl.roles[i] == role && s.w[i] > 0 && (role != RoleStock || l.YieldPct > 0) {
			tot += s.w[i]
		}
	}
	var out []Target
	for i, l := range s.lines {
		if pl.roles[i] == role && s.w[i] > 0 && (role != RoleStock || l.YieldPct > 0) {
			out = append(out, Target{Line: i, Share: s.w[i] / tot})
		}
	}
	return out
}

// split is how much of an amount goes to dividend stocks and to income funds: stockShare to the
// stocks, the rest to the funds; all to one side when the other has no line.
func (pl switchPlan) split(s *yocSim, amount float64) (stocks, funds float64) {
	hasS, hasF := len(pl.targets(s, RoleStock)) > 0, len(pl.targets(s, RoleIncome)) > 0
	switch {
	case hasS && hasF:
		return amount * pl.stockShare / 100, amount * (1 - pl.stockShare/100)
	case hasS:
		return amount, 0
	case hasF:
		return 0, amount
	}
	return 0, 0
}

// buy spends the proceeds left after the bonds on the dividend stocks and income funds and returns
// the FX cost: each currency's proceeds pay FXCostPct when the target trades in another currency.
func (pl switchPlan) buy(s *yocSim, proceeds map[string]float64, sold, bonds float64) float64 {
	if sold <= 0 {
		return 0
	}
	rest := sold - bonds
	toS, toF := pl.split(s, rest)
	var fx float64
	for _, part := range []struct {
		role   string
		amount float64
	}{{RoleStock, toS}, {RoleIncome, toF}} {
		for _, t := range pl.targets(s, part.role) {
			to := strings.ToUpper(s.lines[t.Line].BuyCurrency)
			for c, v := range proceeds {
				amt := part.amount * t.Share * v / sold
				cost := 0.0
				if c != "" && to != "" && c != to {
					cost = amt * FXCostPct() / 100
				}
				fx += cost
				s.units[t.Line] += (amt - cost) / s.price(t.Line, float64(s.k))
			}
		}
	}
	return fx
}

// ProjectSwitch runs the switch for an allocation (filled and normalised the way CheckPortfolio
// does) and its YoC lines.
func ProjectSwitch(a Allocation, lines []YoCLine, p SwitchParams) *SwitchReport {
	p = p.Resolved()
	r := &SwitchReport{Params: p}
	var unset []string
	for _, x := range []struct {
		name string
		ok   bool
	}{{"switch_years", len(p.Years) > 0}, {"switch_sell_pct", p.SellPct != nil}, {"switch_stock_share_pct", p.StockSharePct != nil}, {"bond_cap_pct", p.BondCapPct != nil}} {
		if !x.ok {
			unset = append(unset, x.name)
		}
	}
	if len(unset) > 0 {
		r.Notes = append(r.Notes, "no switch to income projected: "+strings.Join(unset, ", ")+" "+pack.UnsetNote)
		return r
	}
	if len(p.BondYieldsPct) == 0 {
		r.Notes = append(r.Notes, "no bond yields in the country pack (bonds.yields_pct): the switch is projected with the bonds paying 0%")
		p.BondYieldsPct = []float64{0}
		r.Params.BondYieldsPct = p.BondYieldsPct
	}
	roles := make([]string, len(a.Lines))
	for i, l := range a.Lines {
		roles[i] = SwitchRole(l)
		if lines[i].Weight <= 0 {
			continue
		}
		switch roles[i] {
		case RoleSell:
			r.Sold = append(r.Sold, l.Ticker)
		case RoleStock:
			r.StockTargets = append(r.StockTargets, l.Ticker)
		case RoleIncome:
			r.IncomeTargets = append(r.IncomeTargets, l.Ticker)
		}
	}
	sell := *p.SellPct / 100
	pl := switchPlan{roles: roles, sellFrac: 1 - math.Pow(1-sell, 1/float64(p.GlideYears+1)),
		bondCap: *p.BondCapPct, stockShare: *p.StockSharePct, glide: p.GlideYears}
	switch {
	case len(r.Sold) == 0:
		r.Notes = append(r.Notes, "no growth or all-cap line: nothing is sold, the switch only stops the contributions")
	case len(r.StockTargets)+len(r.IncomeTargets) == 0:
		r.Notes = append(r.Notes, "no dividend stock and no income fund to buy: all the proceeds go to bonds")
	case len(r.StockTargets) == 0:
		r.Notes = append(r.Notes, "no dividend stock in the mix: the non-bond proceeds all buy the income funds")
	case len(r.IncomeTargets) == 0:
		r.Notes = append(r.Notes, "no covered-call (income bucket) fund in the mix: the non-bond proceeds all buy the dividend stocks")
	}
	if p.GlideYears > 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("glide: at each of the %d year ends before the switch and at the switch, %.1f%% of the growth holdings is sold (%.0f%% in all); the glide steps buy dividend stocks and income funds, bonds only at the switch; contributions keep buying every line at its weight",
			p.GlideYears, pl.sellFrac*100, *p.SellPct))
	}
	infl := a.InflationPct
	if p.TargetMonthly <= 0 {
		r.Notes = append(r.Notes, "no income target: target_monthly is "+pack.UnsetNote+", so the year it is reached is not searched")
	}
	monthlies := []float64{a.Monthly}
	if p.HigherMonthly > 0 && p.HigherMonthly != a.Monthly {
		monthlies = append(monthlies, p.HigherMonthly)
	}
	for _, m := range monthlies {
		for _, k := range p.Years {
			for _, b := range p.BondYieldsPct {
				full, ok := runSwitch(lines, pl, a.StartAmount, m, k, b)
				if !ok {
					continue
				}
				coh, _ := runSwitch(lines, pl, a.StartAmount, 0, k, b)
				contributed := a.StartAmount + 12*m*float64(k)
				row := SwitchRow{Monthly: m, AfterYears: k, Year: a.StartYear + k, BondYieldPct: b,
					Contributed: round(contributed, 0), Value: round(full.value, 0),
					RealValue: round(full.value/math.Pow(1+infl/100, float64(k)), 0),
					Sold:      round(full.sold, 0), Glided: round(full.glided, 0), Bonds: round(full.bonds, 0),
					ToStocks: round(full.toStocks, 0), ToIncomeFunds: round(full.toIncome, 0), FXCost: round(full.fx, 0),
					MonthlyBefore: round(full.before/12, 0), MonthlyIncome: round(full.after/12, 0),
					MonthlyIncomeReal: round(full.after/12/math.Pow(1+infl/100, float64(k)), 0),
				}
				if p.TargetMonthly > 0 {
					row.TargetGap = round(full.after/12-p.TargetMonthly, 0)
				}
				if contributed > 0 {
					row.YoCPct = round(full.after/contributed*100, 2)
				}
				if a.StartAmount > 0 {
					row.StartMoneyYoCPct = round(coh.after/a.StartAmount*100, 2)
				}
				if full.after > 0 {
					row.Growth10Pct = round((math.Pow(full.after10/full.after, 1.0/switchGrowthHorizonYr)-1)*100, 2)
				}
				row.GrowthStatus = Pass
				if row.Growth10Pct <= infl {
					row.GrowthStatus = Fail
				}
				r.Rows = append(r.Rows, row)
			}
		}
		if p.TargetMonthly > 0 {
			for _, b := range p.BondYieldsPct {
				r.Reach = append(r.Reach, reach(a, lines, pl, m, b, p.TargetMonthly, infl))
			}
		}
	}
	return r
}

// reach finds the first switch year whose net monthly income reaches the target, in future and
// in today's money, searching up to switchSearchMaxYears.
func reach(a Allocation, lines []YoCLine, pl switchPlan, monthly, bond, target, infl float64) SwitchReach {
	out := SwitchReach{Monthly: monthly, BondYieldPct: bond}
	for k := 1; k <= switchSearchMaxYears && out.RealYear == nil; k++ {
		res, ok := runSwitch(lines, pl, a.StartAmount, monthly, k, bond)
		if !ok {
			break
		}
		m := res.after / 12
		if out.Year == nil && m >= target {
			y, n := a.StartYear+k, k
			out.Year, out.AfterYears = &y, &n
		}
		if m/math.Pow(1+infl/100, float64(k)) >= target {
			y, n := a.StartYear+k, k
			out.RealYear, out.RealAfterYears = &y, &n
		}
	}
	if out.Year == nil {
		out.Note = fmt.Sprintf("not reached within %d years", switchSearchMaxYears)
	} else if out.RealYear == nil {
		out.Note = fmt.Sprintf("in today's money not reached within %d years", switchSearchMaxYears)
	}
	return out
}
