package pbcli

// final_test.go - pb final from-file: the page's final mix into allocation_final.yaml.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
)

func TestFinalFromFileWritesTheAllocation(t *testing.T) {
	data := t.TempDir()
	rd := filepath.Join(data, "runs", "2031-03-07")
	_ = os.MkdirAll(rd, 0o755)
	_ = os.WriteFile(filepath.Join(data, "watchlist.yaml"), []byte("as_of: x\nsource: t\nitems:\n  - {ticker: RLTY, kind: stock}\n  - {ticker: MOON, kind: stock}\n  - {ticker: FCCA, kind: etf, currency: GBP}\n"), 0o644)
	mix := `currency: XTS
start_amount: 100000
monthly: 20000
years: 10
start_year: 2031
price_growth_pct: 5
lines:
  - {ticker: RLTY, kind: stock, weight_pct: 40, dividend_growth_pct: 2.9, price_growth_pct: 4.3}
  - {ticker: MOON, kind: stock, weight_pct: 10, moonshot: true, dividend_growth_pct: 2.7}
  - {ticker: FCCA, kind: etf_dist, bucket: income, weight_pct: 50, yield_pct: 10.45, dividend_currency: USD, trading_currency: GBP}
`
	_ = os.WriteFile(filepath.Join(rd, "allocation_income.yaml"), []byte(mix), 0o644)
	// As the page downloads it: O unchanged, MOON's dividend growth overridden, FCCA's absent growth shown as 0,
	// and a name that is not on the watchlist.
	doc := `{"run":"2031-03-07","mix":"income","edited":true,"start_amount":100000,"monthly":15000,"years":20,"inflation_pct":3.2,
"lines":[{"ticker":"RLTY","kind":"stock","bucket":"","weight_pct":35,"dividend_growth_pct":2.9,"price_growth_pct":4.3},
{"ticker":"MOON","kind":"stock","bucket":"","weight_pct":10,"dividend_growth_pct":6,"price_growth_pct":5},
{"ticker":"FCCA","kind":"etf_dist","bucket":"income","weight_pct":45,"dividend_growth_pct":0,"price_growth_pct":5},
{"ticker":"ZZZ","kind":"stock","bucket":"","weight_pct":10,"dividend_growth_pct":0,"price_growth_pct":5}],"saved_at":"x"}`
	p := filepath.Join(t.TempDir(), "final-2031-03-07.json")
	_ = os.WriteFile(p, []byte(doc), 0o600)
	out := run(t, data, "final", "from-file", p)
	if !strings.Contains(out, `"ZZZ"`) || !strings.Contains(out, "allocation_income.yaml") {
		t.Errorf("report: %s", out)
	}
	var a rules.Allocation
	readYAMLFile(t, filepath.Join(rd, "allocation_final.yaml"), &a)
	if a.Monthly != 15000 || a.Years != 20 || a.StartYear != 2031 || a.Currency != "XTS" || len(a.Lines) != 3 {
		t.Fatalf("header or lines: %+v", a)
	}
	rlty, homex, fcca := a.Lines[0], a.Lines[1], a.Lines[2]
	if rlty.WeightPct != 35 || *rlty.DividendGrowthPct != 2.9 || *rlty.PriceGrowthPct != 4.3 {
		t.Errorf("RLTY: %+v", rlty)
	}
	if !homex.Moonshot || *homex.DividendGrowthPct != 6 || homex.PriceGrowthPct != nil {
		t.Errorf("MOON keeps moonshot, takes the growth override, not the default price growth: %+v", homex)
	}
	if fcca.YieldPct == nil || *fcca.YieldPct != 10.45 || fcca.TradingCurrency != "GBP" || fcca.DividendGrowthPct != nil || fcca.PriceGrowthPct != nil {
		t.Errorf("FCCA keeps the cited fields and takes no 0/default override: %+v", fcca)
	}
}
