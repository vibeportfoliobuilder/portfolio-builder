package pbcli

// portfolio.go - pb portfolio check <allocation.yaml>: Jack's allocation against the profile.

import (
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/docfmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

func portfolioCmd(e *env) *cobra.Command {
	c := &cobra.Command{Use: "portfolio", Short: "Check an allocation against the investor profile"}
	var out string
	var sw switchFlags
	check := &cobra.Command{
		Use:   "check <allocation.yaml>",
		Short: "Weights, caps, income by currency, fees, the 10-year yield-on-cost path and tobacco look-through",
		Long: `Reads an allocation (schema in CONTRIBUTING.md), fills each line's missing fields from data/
(derived.yaml for stocks, broker.yaml for ETFs, watchlist.yaml for the trading currency, macro.yaml for FX),
and judges it against YOUR strategy (data/profile/strategy.yaml; ./pb packs prints it). Every limit below
is a strategy key; a key you have not set skips its rule and says so:
  - weights and the ETF/stock split (etf_min_pct, etf_max_pct);
  - caps: stock weight (weight_cap_pct), income per stock (income_cap_pct), per ETF (etf_income_cap_pct),
    per moonshot (moonshot_income_cap_pct);
  - income by currency against currency_income_cap_pct, and every income currency matched by an ETF
    trading in it (a currency in stock_reinvest_currencies is reinvested into its own stocks instead);
  - withholding: a paying line with no rate in the packs is flagged and projected gross;
  - fees: the average monthly trading cost at the broker pack's prices, transaction taxes included,
    FX excluded (fee_max_pct);
  - the 10-year path, dividends reinvested into the fund lines of their currency (reinvest rules):
    net yield on the start money at year 10 (yoc_year10_pct; on all money when there is no start
    amount), the weighted price growth (price_growth_pct, price_growth_tolerance_pct), and
    income-weighted dividend growth above the mix's inflation after year 10. A stock's dividend
    growth comes from its history (projection_growth_haircut, projection_growth_cap_pct); a line
    with no growth rate is projected at 0% and flagged;
  - names your strategy excludes (excluded_tickers) inside the funds' published top holdings.
  - the switch to income at each of switch_years: switch_sell_pct of the growth and all-cap holdings
    (Growth-group or low-yield stocks, growth/all_cap funds) is sold, the payers are kept; the proceeds
    buy the country pack's bonds (in their currency, only at the switch) up to bond_cap_pct of the
    portfolio value, the rest split switch_stock_share_pct / the rest between the mix's own dividend
    stocks and its income-bucket (covered-call) funds; bonds at the pack's yields. Per switch: value,
    net monthly income after it, yield on cost (all money, start money), income growth over the next
    10 years against the mix's inflation (no reinvestment after the switch), and the first year
    target_monthly is reached, at the mix's monthly amount and at higher_monthly. A mix may state
    these under switch: {years, sell_pct, bond_cap_pct, bond_yields_pct, stock_share_pct, glide_years,
    target_monthly, higher_monthly}; the --switch-*, --bond-* and --glide-years flags override it.
The starting yield (gross and net) is reported as a figure and judged by nothing.
Writes its own file beside the allocation: allocation_<x>.yaml -> check_<x>.yaml, any other
<name>.yaml -> check_<name>.yaml (so a probe never overwrites another), or the file --out names.
Example:
  pb portfolio check data/runs/<date>/allocation_balanced.yaml --md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			var a rules.Allocation
			if err := yaml.Unmarshal(b, &a); err != nil {
				return fmt.Errorf("%s: %w", args[0], err)
			}
			a.Switch = sw.apply(cmd, a.Switch)
			filled, r := e.checkAllocation(a)
			doc := map[string]any{"as_of": e.today(), "allocation": args[0], "lines_filled": filled.Lines, "report": r}
			y, err := docfmt.MarshalYAML(doc)
			if err != nil {
				return err
			}
			if out == "" {
				out = checkFileFor(args[0])
			}
			if err := store.WriteFileAtomic(out, y); err != nil {
				return err
			}
			if e.md {
				return e.portfolioMD(r)
			}
			return e.printJSON(r)
		},
	}
	check.Flags().StringVar(&out, "out", "", "write the check here instead of check_<name>.yaml beside the allocation")
	sw.register(check)
	c.AddCommand(check)
	return c
}

// checkFileFor is the check's own file beside its allocation: allocation_x.yaml -> check_x.yaml,
// name.yaml -> check_name.yaml.
func checkFileFor(alloc string) string {
	dir, base := filepath.Split(alloc)
	stem := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
	stem = strings.TrimPrefix(stem, "allocation_")
	return filepath.Join(dir, "check_"+stem+".yaml")
}

// checkAllocation fills an allocation from data/ and runs the profile's portfolio rules on it.
func (e *env) checkAllocation(a rules.Allocation) (rules.Allocation, rules.Report) {
	filled := e.fill(a)
	fx := e.fxTable()
	r := rules.CheckPortfolio(filled, func(v float64, to string) (float64, bool) {
		if cur := strings.ToUpper(filled.Currency); cur != "" && cur != pack.Home() {
			rate, ok := fx[cur]
			if !ok {
				return 0, false
			}
			v *= rate
		}
		return fx.fromHome(v, to)
	})
	return filled, r
}

// fill completes each line from data/: a field the allocation states is never overwritten.
func (e *env) fill(a rules.Allocation) rules.Allocation {
	w, _ := e.readWatchlist()
	for i := range a.Lines {
		l := &a.Lines[i]
		dir := dirName(l.Ticker)
		for _, it := range w.Items {
			if strings.EqualFold(it.Ticker, l.Ticker) {
				l.TradingCurrency = firstNonEmpty(l.TradingCurrency, it.Currency)
				l.ISIN = firstNonEmpty(l.ISIN, it.ISIN)
				l.Country = firstNonEmpty(l.Country, it.Country)
				if l.Kind == "" && it.Kind == kindETF {
					l.Kind = rules.KindETFDist
				}
			}
		}
		if l.Kind == "" {
			l.Kind = rules.KindStock
		}
		if l.IsETF() {
			var ly broker.ETF
			if ok, _ := e.readYAML(&ly, "etfs", dir, broker.File); ok && ly.Fund != nil {
				f := ly.Fund
				if l.YieldPct == nil && f.YieldPct != nil {
					l.YieldPct = f.YieldPct
				}
				l.DividendCurrency = firstNonEmpty(l.DividendCurrency, f.DividendCurrency)
				l.TradingCurrency = firstNonEmpty(l.TradingCurrency, ly.Instrument.Currency)
				if strings.HasPrefix(strings.ToLower(f.DistributionPolicy), "acc") {
					l.Kind = rules.KindETFAcc
				}
				if l.ExcludedPct == nil && len(f.TopHoldings) > 0 {
					l.ExcludedPct = excludedShare(f.TopHoldings)
				}
			}
			// A fund's payout currency is its share class's, not the line it trades on (a fund can
			// trade in GBP and pay USD): the broker's fund data above, else the class named in Eulerpool's name.
			if l.DividendCurrency == "" && l.Kind == rules.KindETFDist {
				var ed eulerpool.ETFDossier
				if ok, _ := e.readYAML(&ed, "etfs", dir, "financial_data.yaml"); ok {
					l.DividendCurrency = shareClassCurrency(ed.Name)
				}
				if l.DividendCurrency == "" && l.TradingCurrency != "" {
					l.DividendCurrency = strings.ToUpper(l.TradingCurrency)
					l.CurrencyBasis = "dividend currency unknown (no broker fund data, no class in the name): assumed the trading currency; state dividend_currency in the mix"
				}
			}
			continue
		}
		var d Derived
		if ok, _ := e.readYAML(&d, "stocks", dir, "derived.yaml"); ok {
			if l.YieldPct == nil {
				l.YieldPct = d.YieldPct
			}
			if l.DividendGrowthPct == nil {
				l.DividendGrowthPct, l.GrowthBasis = rules.ProjectedDividendGrowth(d.DividendGrowth["10y"], d.DividendGrowth["5y"], d.DividendGrowth["3y"])
			}
			l.Country = firstNonEmpty(l.Country, d.Country)
			l.ISIN = firstNonEmpty(l.ISIN, d.ISIN)
			l.DividendCurrency = firstNonEmpty(l.DividendCurrency, d.DividendCurrency)
			l.Name = firstNonEmpty(l.Name, d.Name)
			l.Group = firstNonEmpty(l.Group, d.Rules.Group)
			if !l.REIT {
				l.REIT = d.Rules.REIT
			}
		}
		l.TradingCurrency = firstNonEmpty(l.TradingCurrency, l.DividendCurrency)
	}
	// Each line carries the price growth it is projected on, so the page uses pb's rates.
	pg := a.PriceGrowthPct
	if s := pack.Active().Strategy.PriceGrowthPct; pg == 0 && s != nil {
		pg = *s
	}
	for i := range a.Lines {
		if a.Lines[i].PriceGrowthPct == nil {
			v := pg
			a.Lines[i].PriceGrowthPct = &v
		}
	}
	return a
}

var classCcy = regexp.MustCompile(`(?i)(?:\(|\b(?:acc|accum|dist|inc|a)[ -]+)(USD|EUR|GBP)\)?\b|\b(USD|EUR|GBP)\)\s*(?:dist|acc)`)

// shareClassCurrency reads the share-class currency a fund's name states ("INC-USD", "(USD)
// Distributing", "Accum A USD"); "" when it names none.
func shareClassCurrency(name string) string {
	m := classCcy.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1] + m[2])
}

// excludedShare is the share of a fund's published top holdings that the strategy excludes; nil
// when the strategy excludes nothing.
func excludedShare(hs []broker.Weight) *float64 {
	ex := pack.Active().Strategy.ExcludedTickers
	if len(ex) == 0 {
		return nil
	}
	var s float64
	for _, h := range hs {
		for t := range ex {
			if strings.EqualFold(strings.TrimSpace(h.Name), t) {
				s += h.Pct
				break
			}
		}
	}
	return &s
}

func firstPtr(ps ...*float64) *float64 {
	for _, p := range ps {
		if p != nil {
			return p
		}
	}
	return nil
}

func (e *env) portfolioMD(r rules.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "## Portfolio check (%s)\n\nETFs %.1f%% · stocks %.1f%% · start yield %s%% (net %s%%) · monthly fee %s%% · excluded names inside funds %s%%\n\n",
		r.Currency, r.ETFPct, r.StockPct, num(r.StartYieldPct, 2), num(r.StartNetYieldPct, 2), num(r.MonthlyFeePct, 3), num(r.ExcludedInFundsPct, 2))
	var rows [][]string
	for _, x := range r.Rules {
		rows = append(rows, []string{x.Name, x.Status, x.Detail})
	}
	b.WriteString(mdTable([]string{"Rule", "Status", "Detail"}, rows))
	rows = nil
	for _, l := range r.Lines {
		rows = append(rows, []string{l.Ticker, l.Kind, fmt.Sprintf("%.1f", l.WeightPct), num(l.YieldPct, 2),
			fmt.Sprintf("%g", l.WithholdingPct), num(l.NetYieldPct, 2), num(l.IncomeSharePct, 1), l.Currency, num(l.FeePct, 3), strings.Join(l.Notes, "; ")})
	}
	b.WriteString("\n" + mdTable([]string{"Ticker", "Kind", "Weight %", "Yield %", "WHT %", "Net %", "Income %", "Div ccy", "Fee %", "Notes"}, rows))
	rows = nil
	for c, v := range r.IncomeByCurrency {
		rows = append(rows, []string{c, fmt.Sprintf("%.1f", v)})
	}
	b.WriteString("\n" + mdTable([]string{"Income currency", "Share %"}, rows))
	rows = nil
	for _, y := range r.Path {
		rows = append(rows, []string{fmt.Sprint(y.Year), fmt.Sprintf("%.0f", y.Contributed), fmt.Sprintf("%.0f", y.Value),
			fmt.Sprintf("%.0f", y.MonthlyIncome), fmt.Sprintf("%.2f", y.YoCPct), fmt.Sprintf("%.2f", y.CohortYoCPct)})
	}
	b.WriteString("\n" + mdTable([]string{"Year", "Paid in", "Value", "Net income / month", "YoC on all %", "YoC on start money %"}, rows))
	if sw := r.Switch; sw != nil {
		switchMD(&b, sw)
	}
	if len(r.Notes) > 0 {
		b.WriteString("\nNotes:\n- " + strings.Join(r.Notes, "\n- ") + "\n")
	}
	_, err := fmt.Fprint(e.out, b.String())
	return err
}

// switchFlags are pb portfolio check's switch-to-income overrides (rules.SwitchParams).
type switchFlags struct {
	years                        []int
	sell, bondCap, stockShare    float64
	bondYields                   []float64
	glide                        int
	targetMonthly, higherMonthly float64
}

func (f *switchFlags) register(c *cobra.Command) {
	c.Flags().IntSliceVar(&f.years, "switch-years", nil, "switch to income after these years of the path (default: your strategy's)")
	c.Flags().Float64Var(&f.sell, "switch-sell-pct", 0, "percent of the growth and all-cap holdings sold at the switch (default: your strategy's)")
	c.Flags().Float64Var(&f.bondCap, "bond-cap-pct", 0, "the country pack's bonds bought at the switch, at most this percent of the portfolio value (default: your strategy's bond_cap_pct)")
	c.Flags().Float64SliceVar(&f.bondYields, "bond-yield", nil, "bond yields to show, percent (default: the pack's yields)")
	c.Flags().Float64Var(&f.stockShare, "switch-stock-share-pct", 0, "percent of the non-bond proceeds into dividend stocks; the rest into covered-call funds (default: your strategy's)")
	c.Flags().IntVar(&f.glide, "glide-years", 0, "move part of the sale into dividend stocks and income funds over the last N years before the switch (0-5; bonds wait for the switch)")
	c.Flags().Float64Var(&f.targetMonthly, "target-monthly", 0, "the net monthly income target, in the mix currency (default: your strategy's target_monthly)")
	c.Flags().Float64Var(&f.higherMonthly, "higher-monthly", 0, "a higher monthly contribution to project beside the mix's own, in the mix currency (default: your strategy's higher_monthly)")
}

// apply lays the flags the user set over the mix's own switch block.
func (f *switchFlags) apply(c *cobra.Command, p *rules.SwitchParams) *rules.SwitchParams {
	var out rules.SwitchParams
	if p != nil {
		out = *p
	}
	set := func(name string) bool { return c.Flags().Changed(name) }
	if set("switch-years") {
		out.Years = f.years
	}
	if set("switch-sell-pct") {
		out.SellPct = &f.sell
	}
	if set("bond-cap-pct") {
		out.BondCapPct = &f.bondCap
	}
	if set("bond-yield") {
		out.BondYieldsPct = f.bondYields
	}
	if set("switch-stock-share-pct") {
		out.StockSharePct = &f.stockShare
	}
	if set("glide-years") {
		out.GlideYears = f.glide
	}
	if set("target-monthly") {
		out.TargetMonthly = f.targetMonthly
	}
	if set("higher-monthly") {
		out.HigherMonthly = f.higherMonthly
	}
	if p == nil && out.Years == nil && out.SellPct == nil && out.BondCapPct == nil && out.BondYieldsPct == nil &&
		out.StockSharePct == nil && out.GlideYears == 0 && out.TargetMonthly == 0 && out.HigherMonthly == 0 {
		return nil
	}
	return &out
}

// switchMD writes the switch-to-income section of the markdown check.
func switchMD(b *strings.Builder, sw *rules.SwitchReport) {
	p := sw.Params
	if p.SellPct == nil || p.StockSharePct == nil || p.BondCapPct == nil || len(sw.Rows) == 0 {
		b.WriteString("\n### Switch to income\n\nNot projected.\n")
		for _, n := range sw.Notes {
			b.WriteString("\n- " + n)
		}
		b.WriteString("\n")
		return
	}
	fmt.Fprintf(b, "\n### Switch to income\n\nSell %.0f%% of %s; bonds up to %.0f%% of the value; the rest %.0f%% dividend stocks (%s) / %.0f%% income funds (%s); glide %d years.\n\n",
		*p.SellPct, listOrNone(sw.Sold), *p.BondCapPct, *p.StockSharePct, listOrNone(sw.StockTargets), 100-*p.StockSharePct, listOrNone(sw.IncomeTargets), p.GlideYears)
	var rows [][]string
	for _, x := range sw.Rows {
		rows = append(rows, []string{fmt.Sprintf("%.0f", x.Monthly), fmt.Sprint(x.Year), fmt.Sprintf("%g", x.BondYieldPct), fmt.Sprintf("%.0f", x.Value),
			fmt.Sprintf("%.0f", x.Bonds), fmt.Sprintf("%.0f", x.MonthlyBefore), fmt.Sprintf("%.0f", x.MonthlyIncome), fmt.Sprintf("%.0f", x.MonthlyIncomeReal),
			fmt.Sprintf("%.2f", x.YoCPct), fmt.Sprintf("%.2f", x.StartMoneyYoCPct), fmt.Sprintf("%.2f %s", x.Growth10Pct, x.GrowthStatus)})
	}
	b.WriteString(mdTable([]string{"Monthly in", "Switch year", "Bond %", "Value", "Bonds", "Net / month before", "Net / month after", "After, today's " + pack.Home(),
		"YoC all money %", "YoC start money %", "Income growth 10y %"}, rows))
	rows = nil
	yr := func(y *int) string {
		if y == nil {
			return "not within 40 years"
		}
		return fmt.Sprint(*y)
	}
	for _, x := range sw.Reach {
		rows = append(rows, []string{fmt.Sprintf("%.0f", x.Monthly), fmt.Sprintf("%g", x.BondYieldPct), yr(x.Year), yr(x.RealYear)})
	}
	if p.TargetMonthly > 0 {
		fmt.Fprintf(b, "\nFirst switch year with %.0f %s a month net:\n\n", p.TargetMonthly, pack.Home())
		b.WriteString(mdTable([]string{"Monthly in", "Bond %", "Future " + pack.Home(), "Today's " + pack.Home()}, rows))
	}
	for _, n := range sw.Notes {
		b.WriteString("\n- " + n)
	}
	if len(sw.Notes) > 0 {
		b.WriteString("\n")
	}
}

func listOrNone(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, ", ")
}
