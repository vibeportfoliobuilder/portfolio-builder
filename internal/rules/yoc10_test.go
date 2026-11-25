package rules

// yoc10_test.go - yoc_year_10 reads year 10 of the path, not its last year.

import (
	"fmt"
	"strings"
	"testing"
)

func TestYoCYear10ReadsTheTenthYearOfALongerPath(t *testing.T) {
	a := sample()
	a.Years = 20
	r := CheckPortfolio(a, homeFX)
	if len(r.Path) != 20 {
		t.Fatalf("path %d", len(r.Path))
	}
	rule := ruleOf(t, r, "yoc_year_10")
	y10 := r.Path[9]
	if !strings.HasPrefix(rule.Detail, "year 2040 (year 10 of the path)") || !strings.Contains(rule.Detail, fmt.Sprintf("%.2f%%", y10.CohortYoCPct)) {
		t.Errorf("rule %+v, year 10 %v", rule, y10.CohortYoCPct)
	}
	// Push the start money's year-10 yield under the 8% target while year 20 is above it: the rule must fail.
	a.Lines = []Line{{Ticker: "X", Kind: KindStock, WeightPct: 50, YieldPct: p(3.5), DividendGrowthPct: p(4), Country: "ZZ", DividendCurrency: "XTS"},
		{Ticker: "F", Kind: KindETFDist, ISIN: "IE0000000031", WeightPct: 50, YieldPct: p(3.5), DividendGrowthPct: p(4), DividendCurrency: "USD", TradingCurrency: "USD"}}
	r = CheckPortfolio(a, homeFX)
	if r.Path[9].CohortYoCPct >= 8 || r.Path[19].CohortYoCPct < 8 {
		t.Fatalf("fixture: year 10 %v, year 20 %v", r.Path[9].CohortYoCPct, r.Path[19].CohortYoCPct)
	}
	if st := ruleOf(t, r, "yoc_year_10").Status; st != Fail {
		t.Errorf("year 10 under target passed (%s): %+v", st, ruleOf(t, r, "yoc_year_10"))
	}
}

func TestYoCYear10OnAShortPathSaysSo(t *testing.T) {
	a := sample()
	a.Years = 5
	r := CheckPortfolio(a, homeFX)
	if d := ruleOf(t, r, "yoc_year_10").Detail; !strings.Contains(d, "only 5 years") {
		t.Errorf("%s", d)
	}
}
