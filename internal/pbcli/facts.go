package pbcli

// facts.go - pb facts: the statement lines the committee's gates 1 and 2 judge by, per stock, from
// financial_data.yaml (and the repaired TTM in derived.yaml), each figure with the field it came
// from. Read-only; the gates' verdicts stay with the agents.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// factYear is one fiscal year (or the TTM) of statement lines; field names are the YAML's own.
type factYear struct {
	FY          int      `yaml:"fy,omitempty"`
	Period      string   `yaml:"period,omitempty"`
	PeriodEnd   string   `yaml:"period_end,omitempty"`
	Revenue     *float64 `yaml:"revenue"`
	OpIncome    *float64 `yaml:"op_income"`
	OpMarginPct *float64 `yaml:"op_margin_pct"`
	FCF         *float64 `yaml:"fcf"`
	FCFMarginPc *float64 `yaml:"fcf_margin_pct"`
	Shares      *float64 `yaml:"diluted_shares,omitempty"`
	DPS         *float64 `yaml:"dps"`
	Cash        *float64 `yaml:"cash_and_st_inv"`
	Debt        *float64 `yaml:"total_debt"`
	Source      string   `yaml:"source"`
	DPSSource   string   `yaml:"dps_source,omitempty"`
}

// factGate is one computed gate figure with its parts and where they came from.
type factGate struct {
	Value  *float64       `yaml:"value,omitempty"`
	Test   string         `yaml:"test,omitempty"`
	Passed *bool          `yaml:"passed,omitempty"`
	Parts  map[string]any `yaml:"parts,omitempty"`
	Source string         `yaml:"source"`
	Note   string         `yaml:"note,omitempty"`
}

// factsDoc is one name's facts.
type factsDoc struct {
	Ticker   string              `yaml:"ticker"`
	Kind     string              `yaml:"kind,omitempty"`
	Note     string              `yaml:"note,omitempty"`
	File     string              `yaml:"file,omitempty"`
	Currency string              `yaml:"currency,omitempty"`
	Units    string              `yaml:"units,omitempty"`
	Years    []factYear          `yaml:"years,omitempty"`
	TTM      *factYear           `yaml:"ttm,omitempty"`
	Gates    map[string]factGate `yaml:"gates,omitempty"`
	Columns  map[string]string   `yaml:"columns,omitempty"`
}

const unavailable = "DATA UNAVAILABLE"

func factsCmd(e *env) *cobra.Command {
	var watch bool
	var tickers string
	var years int
	c := &cobra.Command{
		Use:   "facts <T ...> | --watchlist [--tickers T,…] [--years 5] [--md]",
		Short: "Statement facts for gates 1 and 2: last N fiscal years, 3y revenue CAGR, FCF margin, Rule of 40, each with its source field",
		Long: `Per stock, the last --years fiscal years of data/stocks/<T>/financial_data.yaml (oldest first): revenue,
operating income, operating margin, FCF, FCF margin, diluted shares, dividends per share, cash and
short-term investments, total debt; the TTM (derived.yaml ttm_used, the repaired TTM, where it exists,
else financial_data.yaml ttm.statements); and the committee's gate figures computed from them:
  gate1_revenue_cagr_3y   revenue CAGR from the year three fiscal years before the latest to the latest
  gate1_profitable        latest FCF > 0 and operating margin > 0
  gate1_cash_vs_debt      cash_and_st_inv > total_debt (latest year)
  gate1_tests             how many of the three hold
  gate2_rule_of_40        latest revenue growth % (year on year) + latest FCF margin %; parts shown,
                          with the 3y CAGR as the growth in parts.with_cagr_3y
Every figure names its field (source), e.g. financial_data.yaml years[fiscal_year=2025].statements.revenue,
which pb get reads as written. Amounts are in the file's units (millions). A missing figure is null
(DATA UNAVAILABLE). Funds print "no statements (fund)". Read-only; the verdicts are the agents'.
Examples:
  pb facts KO MSFT --md
  pb facts --watchlist --tickers KO,O --years 3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if years < 1 {
				return fmt.Errorf("--years must be at least 1")
			}
			type name struct{ ticker, kind string }
			var names []name
			switch {
			case watch:
				if len(args) > 0 {
					return fmt.Errorf("give tickers or --watchlist, not both (use --tickers to pick watchlist names)")
				}
				w, err := e.readWatchlist()
				if err != nil {
					return err
				}
				items, err := pickItems(w.Items, tickers)
				if err != nil {
					return err
				}
				for _, it := range items {
					names = append(names, name{it.Ticker, it.Kind})
				}
			case tickers != "":
				return fmt.Errorf("--tickers goes with --watchlist; otherwise name the tickers as arguments")
			case len(args) == 0:
				return fmt.Errorf("name one or more tickers, or pass --watchlist")
			default:
				for _, a := range args {
					names = append(names, name{a, ""})
				}
			}
			var docs []factsDoc
			for _, n := range names {
				docs = append(docs, e.facts(n.ticker, n.kind, years))
			}
			if e.md {
				var b strings.Builder
				for _, d := range docs {
					b.WriteString(factsMarkdown(d))
				}
				_, err := fmt.Fprint(e.out, b.String())
				return err
			}
			return e.printJSON(docs)
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "every name on data/watchlist.yaml")
	c.Flags().StringVar(&tickers, "tickers", "", "with --watchlist: only these names (comma-separated)")
	c.Flags().IntVar(&years, "years", 5, "how many fiscal years to print")
	return c
}

// facts builds one name's facts; a fund, or a stock without its file, is a note.
func (e *env) facts(ticker, kind string, n int) factsDoc {
	t := dirName(ticker)
	d := factsDoc{Ticker: t}
	exists := func(p string) bool { _, err := os.Stat(e.path(p)); return err == nil }
	if kind == "" && !exists(filepath.Join("stocks", t)) && exists(filepath.Join("etfs", t)) {
		kind = kindETF
	}
	if kind == kindETF {
		d.Kind, d.Note = kindETF, "no statements (fund)"
		return d
	}
	d.Kind = kindStock
	d.File = filepath.ToSlash(filepath.Join("stocks", t, "financial_data.yaml"))
	v, err := e.readUnder(d.File)
	if err != nil {
		d.Note = unavailable + ": no " + d.File
		return d
	}
	d.Currency, _ = dig(v, "currency").(string)
	d.Units, _ = dig(v, "units").(string)
	list, _ := dig(v, "years").([]any)
	var all []factYear
	for _, el := range list {
		st, ok := dig(el, "statements").(map[string]any)
		fy, okFY := asInt(dig(el, "fiscal_year"))
		if !ok || !okFY {
			continue
		}
		y := statementYear(st)
		y.FY = fy
		y.PeriodEnd, _ = dig(el, "period_end").(string)
		y.Source = fmt.Sprintf("financial_data.yaml years[fiscal_year=%d].statements", fy)
		y.DPS = numAt(v, fmt.Sprintf("dividend_history.by_fy[fiscal_year=%d].total", fy))
		if y.DPS != nil {
			y.DPSSource = fmt.Sprintf("financial_data.yaml dividend_history.by_fy[fiscal_year=%d].total", fy)
		}
		all = append(all, y)
	}
	if len(all) == 0 {
		d.Note = unavailable + ": " + d.File + " has no years[] with statements"
		return d
	}
	d.Years = all[max(0, len(all)-n):]
	d.TTM = e.factsTTM(t, v)
	d.Gates = gates(all)
	d.Columns = map[string]string{
		"statement lines": "the row's source + the key (revenue, op_income, fcf, diluted_shares, cash_and_st_inv, total_debt)",
		"op_margin_pct":   "computed: op_income / revenue × 100",
		"fcf_margin_pct":  "computed: fcf / revenue × 100",
		"dps":             "dps_source (per share; specials are not flagged)",
	}
	if d.TTM != nil && d.TTM.FCF != nil {
		for _, k := range []string{"gate1_profitable", "gate2_rule_of_40"} {
			if g := d.Gates[k]; g.Note != "" {
				g.Note += fmt.Sprintf("; the TTM has fcf %s (%s)", fnum(d.TTM.FCF), d.TTM.Source)
				d.Gates[k] = g
			}
		}
	}
	return d
}

// factsTTM is the repaired TTM in derived.yaml (ttm_used) where it exists, else the vendor TTM.
func (e *env) factsTTM(t string, fd any) *factYear {
	src := "derived.yaml ttm_used"
	var st map[string]any
	if dv, err := e.readUnder(filepath.Join("stocks", t, "derived.yaml")); err == nil {
		st, _ = dig(dv, "ttm_used").(map[string]any)
	}
	if st == nil {
		src = "financial_data.yaml ttm.statements"
		st, _ = dig(fd, "ttm.statements").(map[string]any)
	}
	if st == nil {
		return nil
	}
	y := statementYear(st)
	y.Period = "TTM"
	y.PeriodEnd, _ = dig(fd, "ttm.period_end").(string)
	y.Source = src
	y.Shares = nil // a TTM carries no share count
	if y.DPS = numAt(fd, "dividend_history.ttm_per_share"); y.DPS != nil {
		y.DPSSource = "financial_data.yaml dividend_history.ttm_per_share"
	}
	return &y
}

// statementYear reads the statement lines and computes the two margins.
func statementYear(st map[string]any) factYear {
	y := factYear{
		Revenue:  numAt(st, "revenue"),
		OpIncome: numAt(st, "op_income"),
		FCF:      numAt(st, "fcf"),
		Shares:   numAt(st, "diluted_shares"),
		Cash:     numAt(st, "cash_and_st_inv"),
		Debt:     numAt(st, "total_debt"),
	}
	y.OpMarginPct = pctOf(y.OpIncome, y.Revenue)
	y.FCFMarginPc = pctOf(y.FCF, y.Revenue)
	return y
}

// gates computes the gate 1 and gate 2 figures from every year with statements (oldest first).
func gates(all []factYear) map[string]factGate {
	last := all[len(all)-1]
	byFY := map[int]factYear{}
	for _, y := range all {
		byFY[y.FY] = y
	}
	rev := func(fy int) string {
		return fmt.Sprintf("financial_data.yaml years[fiscal_year=%d].statements.revenue", fy)
	}
	out := map[string]factGate{}
	passed := 0
	known := 0
	count := func(b *bool) {
		if b != nil {
			known++
			if *b {
				passed++
			}
		}
	}

	// Gate 1a: three fiscal years of reported revenue growth.
	g := factGate{Test: "revenue CAGR > 0", Source: "computed: (revenue latest / revenue three years earlier)^(1/3) − 1 from " + rev(last.FY-3) + " and " + rev(last.FY)}
	if base, ok := byFY[last.FY-3]; ok && base.Revenue != nil && last.Revenue != nil && *base.Revenue > 0 && *last.Revenue > 0 {
		g.Value = round2((math.Pow(*last.Revenue / *base.Revenue, 1.0/3) - 1) * 100)
		g.Parts = map[string]any{fmt.Sprintf("revenue_fy%d", base.FY): *base.Revenue, fmt.Sprintf("revenue_fy%d", last.FY): *last.Revenue}
		g.Passed = boolp(*g.Value > 0)
	} else {
		g.Note = fmt.Sprintf("%s: needs a positive revenue for FY%d and FY%d", unavailable, last.FY-3, last.FY)
	}
	count(g.Passed)
	cagr := g.Value
	out["gate1_revenue_cagr_3y_pct"] = g

	// Gate 1b: profitable on FCF and operating margin.
	g = factGate{Test: "fcf > 0 and op_margin_pct > 0", Source: fmt.Sprintf("financial_data.yaml years[fiscal_year=%d].statements.fcf and op_income / revenue", last.FY)}
	if last.FCF != nil && last.OpMarginPct != nil {
		g.Parts = map[string]any{"fy": last.FY, "fcf": *last.FCF, "op_margin_pct": *last.OpMarginPct}
		g.Passed = boolp(*last.FCF > 0 && *last.OpMarginPct > 0)
	} else {
		g.Note = fmt.Sprintf("%s: FCF or operating margin missing for FY%d", unavailable, last.FY)
	}
	count(g.Passed)
	out["gate1_profitable"] = g

	// Gate 1c: more cash than debt.
	g = factGate{Test: "cash_and_st_inv > total_debt", Source: fmt.Sprintf("financial_data.yaml years[fiscal_year=%d].statements.cash_and_st_inv and .total_debt", last.FY)}
	if last.Cash != nil && last.Debt != nil {
		g.Parts = map[string]any{"fy": last.FY, "cash_and_st_inv": *last.Cash, "total_debt": *last.Debt}
		g.Passed = boolp(*last.Cash > *last.Debt)
	} else {
		g.Note = fmt.Sprintf("%s: cash or debt missing for FY%d", unavailable, last.FY)
	}
	count(g.Passed)
	out["gate1_cash_vs_debt"] = g

	tests := float64(passed)
	out["gate1_tests"] = factGate{Value: &tests, Test: "passed of the three tests above (2 of 3 passes gate 1)",
		Parts: map[string]any{"passed": passed, "known": known}, Source: "computed: the three gate1_ tests"}

	// Gate 2: Rule of 40 = revenue growth % + FCF margin %.
	g = factGate{Test: "revenue growth % + FCF margin % >= 40",
		Source: fmt.Sprintf("computed: (revenue FY%d / revenue FY%d − 1) × 100 + fcf / revenue × 100, from %s, %s and years[fiscal_year=%d].statements.fcf", last.FY, last.FY-1, rev(last.FY-1), rev(last.FY), last.FY)}
	var growth *float64
	if prev, ok := byFY[last.FY-1]; ok && prev.Revenue != nil && last.Revenue != nil && *prev.Revenue > 0 {
		growth = round2((*last.Revenue / *prev.Revenue - 1) * 100)
	}
	if growth != nil && last.FCFMarginPc != nil {
		g.Value = round2(*growth + *last.FCFMarginPc)
		g.Parts = map[string]any{"revenue_growth_pct": *growth, "fcf_margin_pct": *last.FCFMarginPc}
		if cagr != nil {
			g.Parts["with_cagr_3y"] = map[string]any{"revenue_cagr_3y_pct": *cagr, "score": *round2(*cagr + *last.FCFMarginPc)}
		}
		g.Passed = boolp(*g.Value >= 40)
	} else {
		g.Note = fmt.Sprintf("%s: needs revenue for FY%d and FY%d and FCF for FY%d", unavailable, last.FY-1, last.FY, last.FY)
	}
	out["gate2_rule_of_40"] = g
	return out
}

// factsMarkdown is one name's facts as a table and the gate lines.
func factsMarkdown(d factsDoc) string {
	var b strings.Builder
	if d.Note != "" {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", d.Ticker, d.Note)
		return b.String()
	}
	fmt.Fprintf(&b, "## %s (%s, %s)\n\nUnits: %s\n\n", d.Ticker, d.File, d.Currency, d.Units)
	head := []string{"FY", "Revenue", "Op income", "Op margin %", "FCF", "FCF margin %", "Diluted shares", "DPS", "Cash+ST inv", "Total debt", "Source"}
	row := func(label string, y factYear) []string {
		src := strings.TrimPrefix(y.Source, "financial_data.yaml ")
		return []string{label, fnum(y.Revenue), fnum(y.OpIncome), fnum(y.OpMarginPct), fnum(y.FCF), fnum(y.FCFMarginPc), fnum(y.Shares), fnum(y.DPS), fnum(y.Cash), fnum(y.Debt), src}
	}
	var rows [][]string
	for _, y := range d.Years {
		rows = append(rows, row(fmt.Sprintf("%d", y.FY), y))
	}
	if d.TTM != nil {
		rows = append(rows, row("TTM "+d.TTM.PeriodEnd, *d.TTM))
	}
	b.WriteString(mdTable(head, rows))
	b.WriteString("\nSource: the row's Source in financial_data.yaml (unless it names derived.yaml), key = the column (revenue, op_income, fcf, " +
		"diluted_shares, cash_and_st_inv, total_debt); DPS = dividend_history.by_fy[fiscal_year=FY].total (TTM: dividend_history.ttm_per_share); " +
		"margins computed as op_income or fcf / revenue × 100.\n\nGates:\n")
	for _, k := range []string{"gate1_revenue_cagr_3y_pct", "gate1_profitable", "gate1_cash_vs_debt", "gate1_tests", "gate2_rule_of_40"} {
		g := d.Gates[k]
		fmt.Fprintf(&b, "- %s:", k)
		if g.Value != nil || g.Passed == nil {
			fmt.Fprintf(&b, " %s", fnum(g.Value))
		}
		if g.Passed != nil {
			fmt.Fprintf(&b, " [%s: %v]", g.Test, *g.Passed)
		} else if k == "gate1_tests" {
			fmt.Fprintf(&b, " passed of %v known (2 of 3 passes gate 1)", g.Parts["known"])
		}
		if len(g.Parts) > 0 && k != "gate1_tests" {
			fmt.Fprintf(&b, "; parts %s", cell(g.Parts))
		}
		if g.Note != "" {
			fmt.Fprintf(&b, "; %s", g.Note)
		}
		fmt.Fprintf(&b, "; source %s\n", g.Source)
	}
	b.WriteString("\n")
	return b.String()
}

// numAt reads a number at a pb get path; anything else is nil.
func numAt(v any, path string) *float64 {
	x := dig(v, path)
	if l, ok := x.([]any); ok { // a filter's result
		if len(l) != 1 {
			return nil
		}
		x = l[0]
	}
	switch n := x.(type) {
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case uint64:
		f := float64(n)
		return &f
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil
		}
		return &n
	}
	return nil
}

func asInt(v any) (int, bool) {
	if f := numAt(map[string]any{"v": v}, "v"); f != nil {
		return int(*f), true
	}
	return 0, false
}

// pctOf is a / b × 100, nil when either is missing or b is not positive.
func pctOf(a, b *float64) *float64 {
	if a == nil || b == nil || *b <= 0 {
		return nil
	}
	return round2(*a / *b * 100)
}

func round2(f float64) *float64 {
	r := math.Round(f*100) / 100
	return &r
}

func boolp(b bool) *bool { return &b }

// fnum prints a figure for a table: as stored, or "–" when missing.
func fnum(p *float64) string {
	if p == nil {
		return "–"
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}
