package rules

import (
	"strings"
	"testing"
)

// homeFX is a made-up fixed table: 1 USD = 1.5 XTS, 1 EUR = 1.6 XTS, 1 GBP = 1.9 XTS.
func homeFX(amount float64, to string) (float64, bool) {
	rate := map[string]float64{"XTS": 1, "USD": 1.5, "EUR": 1.6, "GBP": 1.9}[to]
	if rate == 0 {
		return 0, false
	}
	return amount / rate, true
}

func ruleOf(t *testing.T, r Report, name string) Rule {
	t.Helper()
	for _, x := range r.Rules {
		if x.Name == name {
			return x
		}
	}
	t.Fatalf("no rule %s in %+v", name, r.Rules)
	return Rule{}
}

// sample is a made-up mix: invented tickers, amounts and rates (the fund ISINs only carry a domicile).
func sample() Allocation {
	return Allocation{Currency: "XTS", StartAmount: 100_000, Monthly: 20_000, Years: 10, StartYear: 2031,
		Lines: []Line{
			{Ticker: "RLTY", Kind: KindStock, WeightPct: 10, YieldPct: p(5.5), DividendGrowthPct: p(3), Country: "US", REIT: true, DividendCurrency: "USD", TradingCurrency: "USD", Frequency: "monthly"},
			{Ticker: "HOMEA", Kind: KindStock, WeightPct: 10, YieldPct: p(5.4), DividendGrowthPct: p(9), Country: "ZZ", DividendCurrency: "XTS", TradingCurrency: "XTS", Frequency: "annual"},
			{Ticker: "GBBK", Kind: KindStock, WeightPct: 10, YieldPct: p(5), DividendGrowthPct: p(7), Country: "GB", DividendCurrency: "GBP", TradingCurrency: "GBP"},
			{Ticker: "DEUT", Kind: KindStock, WeightPct: 10, YieldPct: p(3.5), DividendGrowthPct: p(6), Country: "DE", DividendCurrency: "EUR", TradingCurrency: "EUR"},
			{Ticker: "HOMEB", Kind: KindStock, WeightPct: 5, YieldPct: p(9), DividendGrowthPct: p(5), Country: "ZZ", DividendCurrency: "XTS", Moonshot: true},
			{Ticker: "TWSC", Kind: KindStock, WeightPct: 10, YieldPct: p(1.2), DividendGrowthPct: p(12), Country: "TW", DividendCurrency: "USD", TradingCurrency: "USD", PriceGrowthPct: p(9)},
			{Ticker: "FINC", Kind: KindETFDist, ISIN: "IE0000000011", WeightPct: 20, YieldPct: p(10), DividendGrowthPct: p(0), DividendCurrency: "USD", TradingCurrency: "USD", Frequency: "monthly", ExcludedPct: p(0)},
			{Ticker: "FWLD", Kind: KindETFDist, ISIN: "IE0000000012", WeightPct: 19, YieldPct: p(1.4), DividendGrowthPct: p(5), DividendCurrency: "USD", TradingCurrency: "EUR", ExcludedPct: p(0.5)},
			{Ticker: "FUKD", Kind: KindETFDist, ISIN: "IE0000000013", WeightPct: 6, YieldPct: p(6), DividendGrowthPct: p(3), DividendCurrency: "GBP", TradingCurrency: "GBP", ExcludedPct: p(9)},
		}}
}

func TestThePortfolioCheckReadsEveryRule(t *testing.T) {
	r := CheckPortfolio(sample(), homeFX)
	if r.ETFPct != 45 || r.StockPct != 55 || ruleOf(t, r, "etf_share").Status != Pass {
		t.Errorf("split: %v %v %+v", r.ETFPct, r.StockPct, ruleOf(t, r, "etf_share"))
	}
	if r.StartYieldPct == nil || *r.StartYieldPct < 4 || *r.StartYieldPct > 7 {
		t.Errorf("start yield %v", r.StartYieldPct)
	}
	if ruleOf(t, r, "currency_match").Status != Pass {
		t.Errorf("currency match: %+v %v %v", ruleOf(t, r, "currency_match"), r.IncomeByCurrency, r.ETFCurrencies)
	}
	if g := r.IncomeByCurrency["GBP"]; g <= 0 || ruleOf(t, r, "gbp_income").Status != Pass {
		t.Errorf("GBP income %v %+v", g, ruleOf(t, r, "gbp_income"))
	}
	if r.MonthlyFeePct == nil || *r.MonthlyFeePct <= 0 || *r.MonthlyFeePct > 0.6 || ruleOf(t, r, "fees").Status != Pass {
		t.Errorf("fee %v %+v", r.MonthlyFeePct, ruleOf(t, r, "fees"))
	}
	if len(r.Path) != 10 || r.Path[9].Year != 2040 || r.Path[9].Contributed != 100_000+20_000*12*10 {
		t.Fatalf("path: %+v", r.Path)
	}
	if r.Path[9].CohortYoCPct <= r.Path[0].CohortYoCPct {
		t.Errorf("yield on the start money did not grow: %v -> %v", r.Path[0].CohortYoCPct, r.Path[9].CohortYoCPct)
	}
	if r.Path[9].Reinvested <= 0 {
		t.Error("nothing was reinvested into the ETFs")
	}
	if r.ExcludedInFundsPct == nil || *r.ExcludedInFundsPct < 0.6 || *r.ExcludedInFundsPct > 0.7 {
		t.Errorf("excluded names inside the funds %v", r.ExcludedInFundsPct)
	}
	if st := ruleOf(t, r, "yoc_year_10").Status; st != Pass && st != Fail {
		t.Errorf("yoc_year_10 is %s: every line has a growth and a withholding rate", st)
	}
	if ruleOf(t, r, "withholding").Status != Pass || ruleOf(t, r, "growth_after_10").Status != Pass {
		t.Errorf("withholding %+v, growth %+v", ruleOf(t, r, "withholding"), ruleOf(t, r, "growth_after_10"))
	}
}

func TestCapsAndCurrencyBreachesFail(t *testing.T) {
	a := sample()
	a.Lines[0].WeightPct = 30 // RLTY at ~26%: weight and income cap
	a.Lines = a.Lines[:len(a.Lines)-1]
	a.Lines[4].YieldPct = p(30) // the moonshot dominates income
	r := CheckPortfolio(a, homeFX)
	for _, name := range []string{"weight_cap", "income_cap", "moonshot_income_cap", "currency_match"} {
		if ruleOf(t, r, name).Status != Fail {
			t.Errorf("%s: %+v", name, ruleOf(t, r, name))
		}
	}
	if !strings.Contains(ruleOf(t, r, "currency_match").Detail, "GBP") {
		t.Errorf("GBP income with no GBP ETF: %+v", ruleOf(t, r, "currency_match"))
	}
}

func TestTheETFShareMustBeInTheBand(t *testing.T) {
	a := Allocation{Lines: []Line{{Ticker: "A", Kind: KindStock, WeightPct: 90, YieldPct: p(5), Country: "ZZ", DividendCurrency: "XTS"},
		{Ticker: "B", Kind: KindETFAcc, WeightPct: 10, TradingCurrency: "EUR"}}}
	if ruleOf(t, CheckPortfolio(a, homeFX), "etf_share").Status != Fail {
		t.Error("10% ETFs passed")
	}
}

func TestTheHomeLevyCountsInTheFee(t *testing.T) {
	a := Allocation{Monthly: 800, Lines: []Line{{Ticker: "HOMEC", Kind: KindStock, WeightPct: 100, YieldPct: p(4), Country: "ZZ", DividendCurrency: "XTS"}}}
	r := CheckPortfolio(a, homeFX)
	// 3 XTS minimum commission (0.375%) + the 0.3% levy = 0.675%: over the 0.6% limit.
	if r.MonthlyFeePct == nil || *r.MonthlyFeePct < 0.67 || *r.MonthlyFeePct > 0.68 || ruleOf(t, r, "fees").Status != Fail {
		t.Errorf("home-market fee %v %+v", r.MonthlyFeePct, ruleOf(t, r, "fees"))
	}
}

func TestDefaultMarkets(t *testing.T) {
	for _, tc := range []struct {
		l    Line
		want string
	}{
		{Line{Kind: KindETFDist}, "etf-xetra"},
		{Line{Kind: KindStock, Country: "TW", TradingCurrency: "USD"}, "us"},
		{Line{Kind: KindStock, Country: "ZZ"}, "home"},
		{Line{Kind: KindStock, Country: "GB"}, "lse"},
	} {
		if got := DefaultMarket(tc.l); got != tc.want {
			t.Errorf("%+v: %s want %s", tc.l, got, tc.want)
		}
	}
}
