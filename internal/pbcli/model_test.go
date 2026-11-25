package pbcli

// model_test.go - the Portfolio page's projection (the <pb-model> block of the report template) run in
// node against pb's own (rules.CheckPortfolio): the yield-on-cost path and the switch to income must
// agree. Skipped where node is not installed.

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack/packtest"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/report"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
)

func modelBlock(t *testing.T) string {
	t.Helper()
	page, err := report.Render(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	a := bytes.Index(page, []byte("// <pb-model>"))
	b := bytes.Index(page, []byte("// </pb-model>"))
	if a < 0 || b < a {
		t.Fatal("the template has no <pb-model> block")
	}
	return string(page[a:b])
}

func pf(v float64) *float64 { return &v }

// modelMix is a made-up mix: invented tickers, amounts and rates.
func modelMix() rules.Allocation {
	return rules.Allocation{Currency: "XTS", StartAmount: 100_000, Monthly: 20_000, Years: 10, StartYear: 2031, InflationPct: 3.2,
		Lines: []rules.Line{
			{Ticker: "RLTY", Kind: rules.KindStock, WeightPct: 9, YieldPct: pf(5.5), DividendGrowthPct: pf(2.9), PriceGrowthPct: pf(4.3), Country: "US", REIT: true, DividendCurrency: "USD", TradingCurrency: "USD", Group: "Stable Grower"},
			{Ticker: "HOMEA", Kind: rules.KindStock, WeightPct: 9, YieldPct: pf(6.1), DividendGrowthPct: pf(8), Country: "ZZ", DividendCurrency: "XTS", TradingCurrency: "XTS", Group: "GOAT"},
			{Ticker: "GBIN", Kind: rules.KindStock, WeightPct: 4, YieldPct: pf(7.5), DividendGrowthPct: pf(1.9), Country: "GB", DividendCurrency: "GBP", TradingCurrency: "GBP", Group: "Yielder"},
			{Ticker: "HOMEB", Kind: rules.KindStock, WeightPct: 3, YieldPct: pf(9), DividendGrowthPct: pf(3), Country: "ZZ", DividendCurrency: "XTS", Moonshot: true},
			{Ticker: "SOFT", Kind: rules.KindStock, WeightPct: 6, YieldPct: pf(0.7), DividendGrowthPct: pf(10), PriceGrowthPct: pf(8), Country: "US", DividendCurrency: "USD", TradingCurrency: "USD", Group: "Grower"},
			{Ticker: "NETW", Kind: rules.KindStock, WeightPct: 5, PriceGrowthPct: pf(11), Country: "US", TradingCurrency: "USD", Group: "Growth"},
			{Ticker: "FCCA", Kind: rules.KindETFDist, ISIN: "IE0000000041", Bucket: "income", WeightPct: 25, YieldPct: pf(10.4), DividendGrowthPct: pf(0), PriceGrowthPct: pf(3.4), DividendCurrency: "USD", TradingCurrency: "GBP"},
			{Ticker: "FCCB", Kind: rules.KindETFDist, ISIN: "IE0000000042", Bucket: "income", WeightPct: 8, YieldPct: pf(10), DividendGrowthPct: pf(0), DividendCurrency: "USD", TradingCurrency: "USD"},
			{Ticker: "FDIV", Kind: rules.KindETFDist, ISIN: "IE0000000043", Bucket: "dividend_growth", WeightPct: 6, YieldPct: pf(3.2), DividendGrowthPct: pf(5), DividendCurrency: "USD", TradingCurrency: "EUR"},
			{Ticker: "FALL", Kind: rules.KindETFAcc, ISIN: "IE0000000044", Bucket: "all_cap", WeightPct: 15, TradingCurrency: "USD"},
			{Ticker: "FSEM", Kind: rules.KindETFAcc, ISIN: "IE0000000045", Bucket: "growth", WeightPct: 10, PriceGrowthPct: pf(9), TradingCurrency: "USD"},
		}}
}

func fixedFX(amount float64, to string) (float64, bool) {
	rate := map[string]float64{"XTS": 1, "USD": 1.5, "EUR": 1.6, "GBP": 1.9}[to]
	return amount / rate, rate > 0
}

func TestThePageProjectsLikePb(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	t.Cleanup(packtest.Activate)
	glide := modelMix()
	glide.Switch = &rules.SwitchParams{Years: []int{10, 15, 20}, BondCapPct: pf(15), StockSharePct: pf(70), GlideYears: 4, BondYieldsPct: []float64{5}, HigherMonthly: 45_000}
	glide.Lines[4].SwitchRole = "keep" // SOFT kept and bought as a dividend stock
	// "bare": a strategy with no target, no higher contribution, no low-yield line and no
	// stock-reinvest currency, so the page and pb must skip the same parts.
	bare := packtest.Set()
	bare.Strategy.TargetMonthly, bare.Strategy.HigherMonthly, bare.Strategy.LowYieldPct, bare.Strategy.StockReinvestCurrencies = nil, nil, nil, nil
	for _, tc := range []struct {
		name string
		a    rules.Allocation
		set  pack.Set
	}{{"defaults", modelMix(), packtest.Set()}, {"glide", glide, packtest.Set()}, {"bare", modelMix(), bare}} {
		name, a := tc.name, tc.a
		pack.Use(tc.set)
		r := rules.CheckPortfolio(a, fixedFX)
		var page []map[string]any
		for i, l := range a.Lines {
			var y any
			if l.YieldPct != nil {
				y = *l.YieldPct
			}
			g := 0.0
			if l.DividendGrowthPct != nil {
				g = *l.DividendGrowthPct
			}
			pg := 0.0 // the page's DEFAULT_PG: the strategy's price_growth_pct, else 0
			if st := pack.Active().Strategy.PriceGrowthPct; st != nil {
				pg = *st
			}
			if l.PriceGrowthPct != nil {
				pg = *l.PriceGrowthPct
			}
			dccy := l.DividendCurrency
			if dccy == "" {
				dccy = l.TradingCurrency
			}
			page = append(page, map[string]any{"ticker": l.Ticker, "kind": l.Kind, "bucket": l.Bucket, "w": l.WeightPct, "y": y, "wh": r.Lines[i].WithholdingPct,
				"g": g, "pg": pg, "dccy": dccy, "tccy": l.TradingCurrency, "moonshot": l.Moonshot, "group": l.Group, "sw": l.SwitchRole})
		}
		in, _ := json.Marshal(map[string]any{"lines": page, "params": a.Switch, "start": a.StartAmount, "monthly": a.Monthly, "years": a.Years, "startYear": a.StartYear, "infl": a.InflationPct})
		pk, _ := json.Marshal(pack.Active())
		script := "var D = {pack: " + string(pk) + "};\n" + modelBlock(t) + `
var IN = ` + string(in) + `;
var yl = IN.lines.map(yocLine);
var out = { full: projectYoC(yl, IN.start, IN.monthly, IN.years, IN.infl), coh: projectYoC(yl, IN.start, 0, IN.years, IN.infl),
  sw: projectSwitch(yl, IN.lines.map(switchRole), IN.params, IN.start, IN.monthly, IN.startYear, IN.infl) };
process.stdout.write(JSON.stringify(out));
`
		f := filepath.Join(t.TempDir(), "model.js")
		if err := os.WriteFile(f, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		b, err := exec.Command(node, f).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: node: %v\n%s", name, err, b)
		}
		var js struct {
			Full, Coh []struct{ Contributed, Value, Fwd float64 }
			Sw        struct {
				Params rules.SwitchParams
				Rows   []rules.SwitchRow   `json:"rows"`
				Reach  []rules.SwitchReach `json:"target_reached"`
			}
		}
		if err := json.Unmarshal(b, &js); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, b)
		}
		if len(js.Full) != len(r.Path) {
			t.Fatalf("%s: path %d vs %d", name, len(js.Full), len(r.Path))
		}
		for i, y := range r.Path {
			near(t, name+" yoc all money", js.Full[i].Fwd/js.Full[i].Contributed*100, y.YoCPct, 0.01)
			near(t, name+" yoc start money", js.Coh[i].Fwd/a.StartAmount*100, y.CohortYoCPct, 0.01)
			near(t, name+" value", js.Full[i].Value, y.Value, 1)
		}
		sw := r.Switch
		want, _ := json.Marshal(sw.Params)
		got, _ := json.Marshal(js.Sw.Params)
		if !bytes.Equal(want, got) {
			t.Errorf("%s: params page %s pb %s", name, got, want)
		}
		if len(js.Sw.Rows) != len(sw.Rows) || len(js.Sw.Reach) != len(sw.Reach) {
			t.Fatalf("%s: rows %d/%d reach %d/%d", name, len(js.Sw.Rows), len(sw.Rows), len(js.Sw.Reach), len(sw.Reach))
		}
		for i, x := range sw.Rows {
			p := js.Sw.Rows[i]
			if p.Year != x.Year || p.Monthly != x.Monthly || p.BondYieldPct != x.BondYieldPct {
				t.Fatalf("%s row %d: page %+v pb %+v", name, i, p, x)
			}
			for _, c := range [][3]float64{{p.Value, x.Value, 1}, {p.Sold, x.Sold, 1}, {p.Glided, x.Glided, 1}, {p.Bonds, x.Bonds, 1}, {p.ToStocks, x.ToStocks, 1},
				{p.ToIncomeFunds, x.ToIncomeFunds, 1}, {p.FXCost, x.FXCost, 1}, {p.MonthlyBefore, x.MonthlyBefore, 1}, {p.MonthlyIncome, x.MonthlyIncome, 1},
				{p.MonthlyIncomeReal, x.MonthlyIncomeReal, 1}, {p.YoCPct, x.YoCPct, 0.01}, {p.StartMoneyYoCPct, x.StartMoneyYoCPct, 0.01}, {p.Growth10Pct, x.Growth10Pct, 0.01}} {
				near(t, name+" switch row", c[0], c[1], c[2])
			}
		}
		for i, x := range sw.Reach {
			p := js.Sw.Reach[i]
			if !sameInt(p.Year, x.Year) || !sameInt(p.RealYear, x.RealYear) {
				t.Errorf("%s reach %d: page %v/%v pb %v/%v", name, i, p.Year, p.RealYear, x.Year, x.RealYear)
			}
		}
	}
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: page %.4f pb %.4f", what, got, want)
	}
}

func sameInt(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
