package rules

// etfincomecap_test.go - income per stock and per fund, each against its own cap (the test
// strategy: 18% and 35%).

import (
	"strings"
	"testing"
)

func TestAFundHasItsOwnIncomeCap(t *testing.T) {
	// Seven equal stocks and one fund paying about 25% of the income: passes as a fund, would fail as a stock.
	var lines []Line
	for _, tk := range []string{"A", "B", "C", "D", "E", "F", "G"} {
		lines = append(lines, Line{Ticker: tk, Kind: KindStock, WeightPct: 10, YieldPct: p(4), Country: "ZZ", DividendCurrency: "XTS"})
	}
	lines = append(lines, Line{Ticker: "INC", Kind: KindETFDist, ISIN: "IE0000000021", WeightPct: 30, YieldPct: p(3.5), DividendCurrency: "USD", TradingCurrency: "USD"})
	r := CheckPortfolio(Allocation{Lines: lines}, homeFX)
	if s := *r.Lines[7].IncomeSharePct; s < 20 || s > 30 {
		t.Fatalf("fixture: fund income share %v", s)
	}
	if c := ruleOf(t, r, "income_cap"); c.Status != Pass {
		t.Errorf("a fund at %.1f%% of income failed: %+v", *r.Lines[7].IncomeSharePct, c)
	}
	// The same share on a stock fails at 18%.
	lines[7].Kind = KindStock
	lines[7].WeightPct = 9
	lines[7].YieldPct = p(15)
	r = CheckPortfolio(Allocation{Lines: lines}, homeFX)
	if c := ruleOf(t, r, "income_cap"); c.Status != Fail || !strings.Contains(c.Detail, "INC") || !strings.Contains(c.Detail, "cap 18%") {
		t.Errorf("a stock over 18%% passed: %+v", c)
	}
	// A fund over 35% fails, and says its cap.
	lines[7] = Line{Ticker: "INC", Kind: KindETFDist, ISIN: "IE0000000021", WeightPct: 50, YieldPct: p(10), DividendCurrency: "USD", TradingCurrency: "USD"}
	r = CheckPortfolio(Allocation{Lines: lines}, homeFX)
	if c := ruleOf(t, r, "income_cap"); c.Status != Fail || !strings.Contains(c.Detail, "cap 35%") {
		t.Errorf("a fund over 35%% passed: %+v", c)
	}
}

func TestGrowthAfterTenNamesTheInflation(t *testing.T) {
	if d := ruleOf(t, CheckPortfolio(sample(), homeFX), "growth_after_10").Detail; !strings.Contains(d, "3.2% inflation") {
		t.Errorf("%s", d)
	}
}
