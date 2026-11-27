package pbcli

// scenario.go - pb scenario: the scenario analyst's calculator. The agent states each case's
// assumptions; pb reads the start values from the data files and does the compounding, so no
// figure is hand-computed. No judgement lives here.

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// ScenarioCase is one case's assumptions, all the agent's: dividend growth and price CAGR in
// percent a year, and an optional one-off dividend cut of CutPct percent from year CutYear on.
type ScenarioCase struct {
	DivGrowth *float64 `yaml:"div_growth" json:"div_growth"`
	PriceCAGR *float64 `yaml:"price_cagr" json:"price_cagr"`
	CutYear   int      `yaml:"cut_year,omitempty" json:"cut_year,omitempty"`
	CutPct    float64  `yaml:"cut_pct,omitempty" json:"cut_pct,omitempty"`
}

// ScenarioFile is the --file form: many names, each with its cases.
type ScenarioFile struct {
	Years int            `yaml:"years,omitempty"`
	Names []ScenarioName `yaml:"names"`
}

// ScenarioName is one name's cases in the --file form (bull, base, bear, or any names).
type ScenarioName struct {
	Ticker string                  `yaml:"ticker"`
	Cases  map[string]ScenarioCase `yaml:"cases"`
}

// StartValue is one start figure and the file and field it was read from.
type StartValue struct {
	Value    float64 `yaml:"value" json:"value"`
	Currency string  `yaml:"currency,omitempty" json:"currency,omitempty"`
	Source   string  `yaml:"source" json:"source"`
}

// ScenarioYear is one year of a case. Dividend figures are absent without a start dividend, net
// ones without a withholding rate.
type ScenarioYear struct {
	Year           int      `yaml:"year" json:"year"`
	Price          float64  `yaml:"price" json:"price"`
	DPS            *float64 `yaml:"dps,omitempty" json:"dps,omitempty"`
	GrossYoCPct    *float64 `yaml:"gross_yoc_pct,omitempty" json:"gross_yoc_pct,omitempty"`
	NetYoCPct      *float64 `yaml:"net_yoc_pct,omitempty" json:"net_yoc_pct,omitempty"`
	CumNetDiv      *float64 `yaml:"cum_net_dividends,omitempty" json:"cum_net_dividends,omitempty"`
	TotalReturnPct *float64 `yaml:"total_return_pct,omitempty" json:"total_return_pct,omitempty"`
	// TotalReturnCAGRPct is the total return a year, compounded (years 1 on).
	TotalReturnCAGRPct *float64 `yaml:"total_return_cagr_pct,omitempty" json:"total_return_cagr_pct,omitempty"`
}

// ScenarioResult is one case's path.
type ScenarioResult struct {
	Case        string         `yaml:"case" json:"case"`
	Assumptions ScenarioCase   `yaml:"assumptions" json:"assumptions"`
	Years       []ScenarioYear `yaml:"years" json:"years"`
}

// ScenarioOut is one name's output: the start values with their sources, references and every case.
type ScenarioOut struct {
	Ticker         string           `yaml:"ticker" json:"ticker"`
	Price          *StartValue      `yaml:"price" json:"price"`
	DPS            *StartValue      `yaml:"dps,omitempty" json:"dps,omitempty"`
	WithholdingPct *StartValue      `yaml:"withholding_pct,omitempty" json:"withholding_pct,omitempty"`
	References     []string         `yaml:"references,omitempty" json:"references,omitempty"`
	Gaps           []string         `yaml:"data_gaps,omitempty" json:"data_gaps,omitempty"`
	Cases          []ScenarioResult `yaml:"cases" json:"cases"`
}

// scenarioConvention is printed with every result, so the agent can state the arithmetic.
const scenarioConvention = "price(n) = price(0) × (1 + price_cagr)^n; dps(n) = dps(0) × (1 + div_growth)^(n−1) for n ≥ 1 " +
	"(year 1 earns today's run rate, as derived.yaml yoc_path does), times (1 − cut_pct) from cut_year on; " +
	"yield on cost = dps(n) / price(0); net = gross × (1 − withholding); cumulative net dividends are years 1..n, " +
	"not reinvested; total return = (price(n) + cumulative net dividends) / price(0) − 1, and a year (1 + total return)^(1/n) − 1."

// parseCase reads "bull:div_growth=6,price_cagr=8,cut_year=3,cut_pct=25".
func parseCase(s string) (string, ScenarioCase, error) {
	name, kv, ok := strings.Cut(s, ":")
	name = strings.ToLower(strings.TrimSpace(name))
	if !ok || name == "" {
		return "", ScenarioCase{}, fmt.Errorf("--case %q: want name:div_growth=..,price_cagr=..[,cut_year=..,cut_pct=..]", s)
	}
	var c ScenarioCase
	for _, part := range strings.Split(kv, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return "", c, fmt.Errorf("--case %s: %q is not key=value", name, part)
		}
		f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), "%"), 64)
		if err != nil {
			return "", c, fmt.Errorf("--case %s: %s=%q is not a number", name, k, v)
		}
		switch strings.TrimSpace(k) {
		case "div_growth":
			c.DivGrowth = &f
		case "price_cagr":
			c.PriceCAGR = &f
		case "cut_year":
			c.CutYear = int(f)
		case "cut_pct":
			c.CutPct = f
		default:
			return "", c, fmt.Errorf("--case %s: unknown key %q (div_growth, price_cagr, cut_year, cut_pct)", name, k)
		}
	}
	return name, c, nil
}

// check refuses an incomplete or impossible case.
func (c ScenarioCase) check(name string, years int) error {
	switch {
	case c.DivGrowth == nil || c.PriceCAGR == nil:
		return fmt.Errorf("case %s: div_growth and price_cagr are both needed", name)
	case *c.PriceCAGR <= -100 || *c.DivGrowth <= -100:
		return fmt.Errorf("case %s: a rate of -100%% or less", name)
	case c.CutPct < 0 || c.CutPct > 100:
		return fmt.Errorf("case %s: cut_pct %g is not 0..100", name, c.CutPct)
	case c.CutPct > 0 && (c.CutYear < 1 || c.CutYear > years):
		return fmt.Errorf("case %s: cut_year %d is not 1..%d", name, c.CutYear, years)
	}
	return nil
}

// scenarioYears are the years shown: 0, 1, 3, 5, 10 within the horizon, and the horizon itself.
func scenarioYears(years int) []int {
	var out []int
	for _, y := range []int{0, 1, 3, 5, 10} {
		if y <= years {
			out = append(out, y)
		}
	}
	if !slices.Contains(out, years) {
		out = append(out, years)
	}
	return out
}

// projectCase compounds one case from the start values; dps and wh are nil when the data lacks them.
func projectCase(price float64, dps, wh *float64, c ScenarioCase, years int) []ScenarioYear {
	g, pc := *c.DivGrowth/100, *c.PriceCAGR/100
	show := scenarioYears(years)
	var out []ScenarioYear
	cum := 0.0
	for n := 0; n <= years; n++ {
		y := ScenarioYear{Year: n, Price: roundN(price*math.Pow(1+pc, float64(n)), 2)}
		if dps != nil {
			d := *dps
			if n >= 1 {
				d = *dps * math.Pow(1+g, float64(n-1))
				if c.CutPct > 0 && n >= c.CutYear {
					d *= 1 - c.CutPct/100
				}
			}
			y.DPS = ptr(roundN(d, 4))
			y.GrossYoCPct = ptr(roundN(d/price*100, 2))
			if wh != nil {
				net := d * (1 - *wh/100)
				if n >= 1 {
					cum += net
				}
				y.NetYoCPct = ptr(roundN(net/price*100, 2))
				y.CumNetDiv = ptr(roundN(cum, 2))
				y.TotalReturnPct = ptr(roundN(((price*math.Pow(1+pc, float64(n))+cum)/price-1)*100, 1))
			}
		}
		if dps == nil || wh == nil {
			y.TotalReturnPct = ptr(roundN((math.Pow(1+pc, float64(n))-1)*100, 1))
		}
		if tr := y.TotalReturnPct; n >= 1 && *tr > -100 {
			y.TotalReturnCAGRPct = ptr(roundN((math.Pow(1+*tr/100, 1/float64(n))-1)*100, 2))
		}
		if slices.Contains(show, n) {
			out = append(out, y)
		}
	}
	return out
}

func roundN(v float64, dp int) float64 {
	p := math.Pow(10, float64(dp))
	return math.Round(v*p) / p
}

func ptr(v float64) *float64 { return &v }

// scenarioStart reads a stock's start values: price and withholding from derived.yaml, the
// dividend per share from financial_data.yaml (in the price's currency, else derived yield × price).
func (e *env) scenarioStart(ticker string) (ScenarioOut, error) {
	out := ScenarioOut{Ticker: ticker}
	var d Derived
	okD, err := e.readYAML(&d, "stocks", ticker, "derived.yaml")
	if err != nil {
		return out, err
	}
	var f eulerpool.Dossier
	okF, err := e.readYAML(&f, "stocks", ticker, "financial_data.yaml")
	if err != nil {
		return out, err
	}
	if !okD && !okF {
		return out, fmt.Errorf("%s: no data/stocks/%s/derived.yaml or financial_data.yaml (run pb ep fetch and pb derive)", ticker, ticker)
	}
	ccy := cmp.Or(d.PriceCurrency, f.PriceCurrency)
	switch {
	case d.Price != nil && *d.Price > 0:
		out.Price = &StartValue{Value: *d.Price, Currency: ccy, Source: "derived.yaml: price (as_of " + d.AsOf + ")"}
	case f.Price.Current != nil && *f.Price.Current > 0:
		out.Price = &StartValue{Value: *f.Price.Current, Currency: ccy, Source: "financial_data.yaml: price.current (price_as_of " + f.Price.AsOf + ")"}
	default:
		return out, fmt.Errorf("%s: no price in derived.yaml or financial_data.yaml: DATA UNAVAILABLE", ticker)
	}
	switch h := f.Dividends; {
	case h != nil && h.TTMPerShare != nil && strings.EqualFold(h.Currency, ccy):
		out.DPS = &StartValue{Value: *h.TTMPerShare, Currency: h.Currency, Source: "financial_data.yaml: dividend_history.ttm_per_share"}
	case d.YieldPct != nil:
		out.DPS = &StartValue{Value: roundN(*d.YieldPct/100*out.Price.Value, 4), Currency: ccy, Source: "derived.yaml: yield_pct × price"}
		if h != nil && h.TTMPerShare != nil {
			out.Gaps = append(out.Gaps, fmt.Sprintf("dividend_history is in %s, the price in %s: the start dividend is derived.yaml yield_pct × price", h.Currency, ccy))
		}
	default:
		out.Gaps = append(out.Gaps, "no dividend per share (financial_data.yaml dividend_history.ttm_per_share, derived.yaml yield_pct): dividend columns DATA UNAVAILABLE")
	}
	if w := d.Rules.Withholding; w != nil {
		out.WithholdingPct = &StartValue{Value: w.RatePct, Source: "derived.yaml: rules.withholding.rate_pct (" + w.Country + ")"}
	} else {
		out.Gaps = append(out.Gaps, "no rules.withholding in derived.yaml (run pb derive): net columns DATA UNAVAILABLE")
	}
	if p := d.YoCPath; p != nil {
		out.References = append(out.References, fmt.Sprintf("derived.yaml yoc_path: growth_pct %g (%s), net yield %g%%", p.GrowthPct, p.GrowthBasis, p.NetYieldPct))
	}
	for _, k := range []string{"3y", "5y", "10y"} {
		if v := d.DividendGrowth[k]; v != nil {
			out.References = append(out.References, fmt.Sprintf("derived.yaml dividend_growth_pct.%s %g", k, *v))
		}
	}
	if hv := d.HumanView; hv != nil && hv.FairValue > 0 {
		out.References = append(out.References, fmt.Sprintf("derived.yaml human_view.fair_value %g %s", hv.FairValue, hv.Currency))
	}
	return out, nil
}

// caseOrder puts bull, base and bear first, then any other case by name.
func caseOrder(m map[string]ScenarioCase) []string {
	rank := map[string]int{"bull": 0, "base": 1, "bear": 2}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		ra, oka := rank[a]
		rb, okb := rank[b]
		switch {
		case oka && okb:
			return ra - rb
		case oka:
			return -1
		case okb:
			return 1
		}
		return strings.Compare(a, b)
	})
	return keys
}

// runScenario projects every case of one name.
func (e *env) runScenario(ticker string, cases map[string]ScenarioCase, years int) (ScenarioOut, error) {
	out, err := e.scenarioStart(dirName(ticker))
	if err != nil {
		return out, err
	}
	var dps, wh *float64
	if out.DPS != nil {
		dps = &out.DPS.Value
	}
	if out.WithholdingPct != nil {
		wh = &out.WithholdingPct.Value
	}
	for _, k := range caseOrder(cases) {
		c := cases[k]
		if err := c.check(k, years); err != nil {
			return out, fmt.Errorf("%s: %w", out.Ticker, err)
		}
		out.Cases = append(out.Cases, ScenarioResult{Case: k, Assumptions: c, Years: projectCase(out.Price.Value, dps, wh, c, years)})
	}
	return out, nil
}

func scenarioCmd(e *env) *cobra.Command {
	var caseArgs []string
	var file string
	var years int
	c := &cobra.Command{
		Use:   "scenario <TICKER ...> --case name:div_growth=..,price_cagr=..[,cut_year=..,cut_pct=..] ... | --file assumptions.yaml",
		Short: "Compound the agent's bull/base/bear assumptions from the data's start price, dividend and withholding",
		Long: `A calculator for the scenario analyst: the assumptions are yours, the start values and the arithmetic
are pb's. Start values: price from derived.yaml (else financial_data.yaml price.current), dividend per
share from financial_data.yaml dividend_history.ttm_per_share (derived.yaml yield_pct × price when the
ledger is in another currency), withholding from derived.yaml rules.withholding.rate_pct; each is
printed with its source. Per case, years 0, 1, 3, 5 and 10 (and the horizon): price, dividend per
share, gross and net yield on cost, cumulative net dividends (not reinvested) and total return.
Case keys: div_growth and price_cagr (percent a year, required), cut_year and cut_pct (a one-off
dividend cut from that year on; growth goes on from the cut level). No network; writes nothing.
The --file form takes many names:
  years: 10
  names:
  - ticker: KO
    cases:
      bull: {div_growth: 6, price_cagr: 7}
      base: {div_growth: 4.5, price_cagr: 4}
      bear: {div_growth: 1, price_cagr: -1, cut_year: 3, cut_pct: 20}
Examples:
  pb scenario KO --case bull:div_growth=6,price_cagr=7 --case base:div_growth=4.5,price_cagr=4 --case bear:div_growth=1,price_cagr=-1,cut_year=3,cut_pct=20 --md
  pb scenario --file data/runs/2026-11-27/scenario_inputs.yaml --md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			type job struct {
				ticker string
				cases  map[string]ScenarioCase
			}
			var jobs []job
			if file != "" {
				if len(args) > 0 || len(caseArgs) > 0 {
					return fmt.Errorf("use either --file or tickers with --case, not both")
				}
				b, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				var sf ScenarioFile
				// Strict: a misspelled key (div_grwoth) is an error, never an assumption silently dropped.
				dec := yaml.NewDecoder(bytes.NewReader(b))
				dec.KnownFields(true)
				if err := dec.Decode(&sf); err != nil {
					return fmt.Errorf("%s: %w", file, err)
				}
				if !cmd.Flags().Changed("years") && sf.Years > 0 {
					years = sf.Years
				}
				for _, n := range sf.Names {
					jobs = append(jobs, job{n.Ticker, n.Cases})
				}
			} else {
				if len(args) == 0 || len(caseArgs) == 0 {
					return fmt.Errorf("name a ticker and at least one --case, or pass --file")
				}
				cases := map[string]ScenarioCase{}
				for _, s := range caseArgs {
					k, c, err := parseCase(s)
					if err != nil {
						return err
					}
					cases[k] = c
				}
				for _, a := range args {
					jobs = append(jobs, job{a, cases})
				}
			}
			if years < 1 || years > 50 {
				return fmt.Errorf("--years %d is not 1..50", years)
			}
			var outs []ScenarioOut
			for _, j := range jobs {
				if len(j.cases) == 0 {
					return fmt.Errorf("%s: no cases", j.ticker)
				}
				o, err := e.runScenario(j.ticker, j.cases, years)
				if err != nil {
					return err
				}
				outs = append(outs, o)
			}
			if !e.md {
				return e.printJSON(map[string]any{"convention": scenarioConvention, "years": years, "names": outs})
			}
			_, err := fmt.Fprint(e.out, scenarioMarkdown(outs))
			return err
		},
	}
	c.Flags().StringArrayVar(&caseArgs, "case", nil, "name:div_growth=..,price_cagr=..[,cut_year=..,cut_pct=..] (repeat per case)")
	c.Flags().StringVar(&file, "file", "", "a YAML file of names and their cases")
	c.Flags().IntVar(&years, "years", 10, "horizon in years")
	return c
}

// scenarioMarkdown is the --md form: start values with sources, then one table row per case and year.
func scenarioMarkdown(outs []ScenarioOut) string {
	var b strings.Builder
	f := func(p *float64, dp int) string {
		if p == nil {
			return "–"
		}
		return strconv.FormatFloat(*p, 'f', dp, 64)
	}
	for _, o := range outs {
		fmt.Fprintf(&b, "## %s\n", o.Ticker)
		fmt.Fprintf(&b, "- price %g %s (%s)\n", o.Price.Value, o.Price.Currency, o.Price.Source)
		if o.DPS != nil {
			fmt.Fprintf(&b, "- dividend per share %g %s (%s)\n", o.DPS.Value, o.DPS.Currency, o.DPS.Source)
		}
		if o.WithholdingPct != nil {
			fmt.Fprintf(&b, "- withholding %g%% (%s)\n", o.WithholdingPct.Value, o.WithholdingPct.Source)
		}
		if len(o.References) > 0 {
			fmt.Fprintf(&b, "- for reference: %s\n", strings.Join(o.References, "; "))
		}
		for _, g := range o.Gaps {
			fmt.Fprintf(&b, "- data gap: %s\n", g)
		}
		var rows [][]string
		for _, c := range o.Cases {
			a := fmt.Sprintf("g %g%%, price %g%%", *c.Assumptions.DivGrowth, *c.Assumptions.PriceCAGR)
			if c.Assumptions.CutPct > 0 {
				a += fmt.Sprintf(", cut %g%% in y%d", c.Assumptions.CutPct, c.Assumptions.CutYear)
			}
			for i, y := range c.Years {
				name := ""
				if i == 0 {
					name = c.Case + " (" + a + ")"
				}
				rows = append(rows, []string{name, strconv.Itoa(y.Year), strconv.FormatFloat(y.Price, 'f', 2, 64), f(y.DPS, 4),
					f(y.GrossYoCPct, 2), f(y.NetYoCPct, 2), f(y.CumNetDiv, 2), f(y.TotalReturnPct, 1), f(y.TotalReturnCAGRPct, 2)})
			}
		}
		b.WriteString(mdTable([]string{"Case", "Year", "Price", "DPS", "YoC gross %", "YoC net %", "Cum. net div", "Total return %", "TR a year %"}, rows))
		b.WriteString("\n")
	}
	b.WriteString("Convention: " + scenarioConvention + "\n")
	return b.String()
}
