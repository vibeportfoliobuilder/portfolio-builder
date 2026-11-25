package rules

// reinvest_test.go - currency-matched reinvestment.

import (
	"strings"
	"testing"
)

func targetsOf(lines []YoCLine, ccy string) map[int]Target {
	out := map[int]Target{}
	for _, t := range ReinvestTargets(lines, ccy) {
		out[t.Line] = t
	}
	return out
}

func TestEachCurrencyBuysItsOwnLines(t *testing.T) {
	lines := []YoCLine{
		{Weight: 10, YieldPct: 5, Currency: "USD", BuyCurrency: "USD"},              // 0 US stock
		{Weight: 10, YieldPct: 5, Currency: "XTS", BuyCurrency: "XTS"},              // 1 home-market stock
		{Weight: 10, YieldPct: 5, Currency: "GBP", BuyCurrency: "GBP"},              // 2 UK stock
		{Weight: 10, YieldPct: 5, Currency: "EUR", BuyCurrency: "EUR"},              // 3 EU stock, no EUR fund
		{Weight: 30, YieldPct: 10, Fund: true, Currency: "USD", BuyCurrency: "USD"}, // 4 USD income fund
		{Weight: 20, Fund: true, Acc: true, BuyCurrency: "USD"},                     // 5 USD accumulating
		{Weight: 10, YieldPct: 10, Fund: true, Currency: "USD", BuyCurrency: "GBP"}, // 6 GBP line paying USD (FCCA)
	}
	usd := targetsOf(lines, "USD")
	if len(usd) != 2 || usd[4].FXCostPct != 0 || usd[5].FXCostPct != 0 {
		t.Errorf("USD targets %+v, want lines 4 and 5 with no FX", usd)
	}
	approx(t, "USD share of the income fund", usd[4].Share, 0.6, 1e-12)
	if g := targetsOf(lines, "GBP"); len(g) != 1 || g[6].Share != 1 || g[6].FXCostPct != 0 {
		t.Errorf("GBP targets %+v, want the GBP line only", g)
	}
	if h := targetsOf(lines, "XTS"); len(h) != 1 || h[1].Share != 1 {
		t.Errorf("XTS targets %+v, want the home-market stock", h)
	}
	// No EUR line: converted into the distributing funds only (never the accumulating one), FX charged.
	e := targetsOf(lines, "EUR")
	if len(e) != 2 || e[4].FXCostPct != FXCostPct() || e[6].FXCostPct != FXCostPct() {
		t.Errorf("EUR targets %+v, want lines 4 and 6 after FX", e)
	}
	if _, ok := e[5]; ok {
		t.Error("an accumulating fund took another currency's income")
	}
}

// A USD stock and a USD fund and a GBP fund, 100 in, no growth: the year's 2 of USD income all buys
// the USD fund (price 1.06), none the GBP fund.
func TestProjectionReinvestsOnlyIntoTheMatchingFund(t *testing.T) {
	lines := []YoCLine{
		{Weight: 50, YieldPct: 4, PriceGrowthPct: 6, Currency: "USD", BuyCurrency: "USD"},
		{Weight: 25, PriceGrowthPct: 6, Fund: true, Acc: true, BuyCurrency: "USD"},
		{Weight: 25, PriceGrowthPct: 6, Fund: true, Acc: true, BuyCurrency: "GBP"},
	}
	p := ProjectYoC(lines, 100, 0, 1, 3.2)
	approx(t, "income", p[0].Income, 2, 1e-12)
	approx(t, "value", p[0].Value, 100*1.06+2, 1e-9)
	// Pooled (no currencies) the same income would also buy the GBP fund; the value is the same, so
	// check the split through a yielding GBP fund instead.
	lines[2].YieldPct, lines[2].Acc, lines[2].Currency = 4, false, "GBP"
	lines[1].YieldPct, lines[1].Acc, lines[1].Currency = 8, false, "USD"
	p = ProjectYoC(lines, 100, 0, 1, 3.2)
	// USD income 2 + 2 = 4 buys line 1 only: 4/1.06 units at 8%. GBP income 1 buys line 2: 1/1.06 units at 4%.
	want := 50*0.04 + (25+4/1.06)*0.08 + (25+1/1.06)*0.04
	approx(t, "forward", p[0].Forward, want, 1e-9)
}

func TestIncomeWithNoMatchingLinePaysFX(t *testing.T) {
	lines := []YoCLine{
		{Weight: 50, YieldPct: 10, PriceGrowthPct: 0, Currency: "EUR", BuyCurrency: "EUR"},
		{Weight: 50, PriceGrowthPct: 0, Fund: true, BuyCurrency: "USD"},
	}
	p := ProjectYoC(lines, 100, 0, 1, 0)
	approx(t, "value", p[0].Value, 100+5*(1-FXCostPct()/100), 1e-9)
}

func TestTheCheckSaysWhereEachCurrencyGoes(t *testing.T) {
	r := CheckPortfolio(sample(), homeFX)
	all := strings.Join(r.Reinvest, " | ")
	for _, want := range []string{"GBP dividends are reinvested into FUKD", "XTS dividends are reinvested into HOMEA, HOMEB", "EUR dividends are reinvested into FWLD", "USD dividends are reinvested into FINC"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in %s", want, all)
		}
	}
}
