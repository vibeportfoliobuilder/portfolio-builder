package pbcli

// ly.go - pb ly screen: Lightyear's public screener (no login), the example screener connector.
// A broker's account connector (built at setup) lives outside this repository.

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/lightyear"
)

func lyCmd(e *env) *cobra.Command {
	c := &cobra.Command{Use: "ly", Short: "Lightyear's public screener (no login): the example screener connector"}
	c.AddCommand(lyScreenCmd(e))
	return c
}

// ScreenStock is one screener row as data/screen/stocks.yaml stores it.
type ScreenStock struct {
	Ticker          string   `yaml:"ticker" json:"ticker"`
	Name            string   `yaml:"name" json:"name"`
	ISIN            string   `yaml:"isin,omitempty" json:"isin,omitempty"`
	MIC             string   `yaml:"mic,omitempty" json:"mic,omitempty"`
	Currency        string   `yaml:"currency,omitempty" json:"currency,omitempty"`
	IssueType       string   `yaml:"issue_type,omitempty" json:"issue_type,omitempty"`
	Country         string   `yaml:"country,omitempty" json:"country,omitempty"`
	Sector          string   `yaml:"sector,omitempty" json:"sector,omitempty"`
	Price           *float64 `yaml:"price,omitempty" json:"price,omitempty"`
	MarketCapUSDm   *float64 `yaml:"market_cap_usd_m,omitempty" json:"market_cap_usd_m,omitempty"`
	PE              *float64 `yaml:"pe,omitempty" json:"pe,omitempty"`
	YieldPct        *float64 `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	PayoutPct       *float64 `yaml:"payout_pct,omitempty" json:"payout_pct,omitempty"`
	DivGrowth3yPct  *float64 `yaml:"div_growth_3y_pct,omitempty" json:"div_growth_3y_pct,omitempty"`
	DivGrowth10yPct *float64 `yaml:"div_growth_10y_pct,omitempty" json:"div_growth_10y_pct,omitempty"`
	EPSGrowth3yPct  *float64 `yaml:"eps_growth_3y_pct,omitempty" json:"eps_growth_3y_pct,omitempty"`
	InstrumentID    string   `yaml:"instrument_id,omitempty" json:"instrument_id,omitempty"`
}

// ScreenFund is one fund row as data/screen/funds.yaml stores it.
type ScreenFund struct {
	Ticker       string `yaml:"ticker" json:"ticker"`
	Name         string `yaml:"name" json:"name"`
	ISIN         string `yaml:"isin,omitempty" json:"isin,omitempty"`
	Exchange     string `yaml:"exchange,omitempty" json:"exchange,omitempty"`
	MIC          string `yaml:"mic,omitempty" json:"mic,omitempty"`
	Currency     string `yaml:"currency,omitempty" json:"currency,omitempty"`
	Class        string `yaml:"class,omitempty" json:"class,omitempty"`
	IssueType    string `yaml:"issue_type,omitempty" json:"issue_type,omitempty"`
	AssetType    string `yaml:"asset_type,omitempty" json:"asset_type,omitempty"`
	Manager      string `yaml:"manager,omitempty" json:"manager,omitempty"`
	InstrumentID string `yaml:"instrument_id,omitempty" json:"instrument_id,omitempty"`
}

type screenFlags struct {
	minYield, maxYield, maxPayout, minMcap, minDivG3, maxPE float64
	countries, currencies, sector, issueType, mic           string
	class, manager, assetType, name                         string
	sort                                                    string
	limit                                                   int
	// appendRows merges the rows into data/screen/stocks.yaml instead of replacing it.
	appendRows bool
}

// Screener is a public stock and fund screener: every row, filtered by pb. The example is
// Lightyear's public explore screener (no login); a broker connector registers its own.
type Screener interface {
	Stocks(ctx context.Context, progress func(have, total int)) ([]lightyear.Stock, error)
	Funds(ctx context.Context, progress func(have, total int)) ([]lightyear.Fund, error)
}

var screeners = map[string]func() (Screener, error){}

// RegisterScreener installs the screener pb ly screen reads while the broker pack named key is
// active; with none registered for it, the public example screener is used.
func RegisterScreener(key string, open func() (Screener, error)) {
	screeners[strings.ToLower(strings.TrimSpace(key))] = open
}

func openScreener() (Screener, error) {
	if open := screeners[strings.ToLower(pack.Active().Broker.Key)]; open != nil {
		return open()
	}
	cl := lightyear.NewClient()
	if ca, err := rawCache(); err == nil {
		cl.Cache = ca
	}
	return cl, nil
}

func lyScreenCmd(e *env) *cobra.Command {
	var f screenFlags
	c := &cobra.Command{
		Use:   "screen stocks|funds",
		Short: "Read the screener (the example: Lightyear's public one, no login) and filter it",
		Long: `Walks the screener your broker connector registered, else the example: Lightyear's public explore screener (stocks: about 6,000 rows; funds: about 4,000) through
the raw cache at os.UserCacheDir()/pb/raw (pages kept about a day), filters it, prints the rows
and writes data/screen/<stocks|funds>.yaml (filtered rows only).

Stock filters: --min-yield --max-yield --max-payout --min-mcap-usd (millions) --min-div-growth-3y
--max-pe --country US,GB --currency USD,EUR --sector Energy --issue-type COMMON_STOCK --mic XNYS.
Fund filters: --class Dist --currency USD --manager iShares --asset-type "Equity ETFs" --name dividend.
--append (stocks) merges the rows into the existing pool instead of replacing it, so a no-floor growth
screen and a yield screen make one pool for pb screen draft (the same listing is kept once).
Examples:
  pb ly screen stocks --min-yield 2 --min-mcap-usd 2000 --limit 0
  pb ly screen stocks --min-mcap-usd 10000 --limit 0 --append      # large caps with no yield floor (growth)
  pb ly screen stocks --min-yield 4 --max-payout 70 --currency USD,EUR --limit 50
  pb ly screen funds --class Dist --currency USD --name "covered call" --md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := openScreener()
			if err != nil {
				return err
			}
			switch args[0] {
			case "stocks":
				return e.screenStocks(cmd.Context(), cl, f)
			case "funds":
				return e.screenFunds(cmd.Context(), cl, f)
			}
			return fmt.Errorf("screen what? stocks or funds, not %q", args[0])
		},
	}
	fl := c.Flags()
	fl.Float64Var(&f.minYield, "min-yield", 0, "minimum dividend yield, percent")
	fl.Float64Var(&f.maxYield, "max-yield", 0, "maximum dividend yield, percent (0 = none)")
	fl.Float64Var(&f.maxPayout, "max-payout", 0, "maximum payout ratio, percent (0 = none)")
	fl.Float64Var(&f.minMcap, "min-mcap-usd", 0, "minimum market cap, millions of USD")
	fl.Float64Var(&f.minDivG3, "min-div-growth-3y", 0, "minimum 3-year dividend growth, percent (0 = none)")
	fl.Float64Var(&f.maxPE, "max-pe", 0, "maximum P/E (0 = none)")
	fl.StringVar(&f.countries, "country", "", "comma-separated company countries (as the screener names them, or alpha-2)")
	fl.StringVar(&f.currencies, "currency", "", "comma-separated listing currencies")
	fl.StringVar(&f.sector, "sector", "", "sector substring")
	fl.StringVar(&f.issueType, "issue-type", "", "COMMON_STOCK, ADR, PREFERRED_STOCK, ETF …")
	fl.StringVar(&f.mic, "mic", "", "comma-separated venue MICs")
	fl.StringVar(&f.class, "class", "", "funds: Dist or Acc")
	fl.StringVar(&f.manager, "manager", "", "funds: manager substring")
	fl.StringVar(&f.assetType, "asset-type", "", "funds: asset type substring")
	fl.StringVar(&f.name, "name", "", "name substring")
	fl.StringVar(&f.sort, "sort", "yield", "stocks: yield, mcap, divgrowth3y or pe")
	fl.IntVar(&f.limit, "limit", 100, "maximum rows printed and written (0 = all)")
	fl.BoolVar(&f.appendRows, "append", false, "stocks: merge into data/screen/stocks.yaml instead of replacing it")
	return c
}

func listSet(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.ToUpper(strings.TrimSpace(x)); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func inSet(set []string, v string) bool {
	return len(set) == 0 || slices.Contains(set, strings.ToUpper(strings.TrimSpace(v)))
}

func has(hay, needle string) bool {
	return needle == "" || strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}

func (e *env) screenStocks(ctx context.Context, cl Screener, f screenFlags) error {
	all, err := cl.Stocks(ctx, nil)
	if err != nil {
		return err
	}
	countries, ccys, mics := listSet(f.countries), listSet(f.currencies), listSet(f.mic)
	var out []ScreenStock
	for _, s := range all {
		switch {
		case !inSet(ccys, s.Currency), !inSet(mics, s.MIC), !has(s.Sector, f.sector), !has(s.Name, f.name):
			continue
		case len(countries) > 0 && !inSet(countries, s.Country) && !inSet(countries, countryCode(s.Country)):
			continue
		case f.issueType != "" && !strings.EqualFold(s.IssueType, f.issueType):
			continue
		case f.minYield > 0 && (s.Yield == nil || *s.Yield < f.minYield):
			continue
		case f.maxYield > 0 && s.Yield != nil && *s.Yield > f.maxYield:
			continue
		case f.maxPayout > 0 && (s.PayoutRatio == nil || *s.PayoutRatio > f.maxPayout):
			continue
		case f.minMcap > 0 && (s.MarketCapUSD == nil || *s.MarketCapUSD/1e6 < f.minMcap):
			continue
		case f.minDivG3 > 0 && (s.DivGrowth3y == nil || *s.DivGrowth3y < f.minDivG3):
			continue
		case f.maxPE > 0 && (s.PE == nil || *s.PE > f.maxPE):
			continue
		}
		row := ScreenStock{Ticker: s.Ticker, Name: s.Name, ISIN: s.ISIN, MIC: s.MIC, Currency: s.Currency,
			IssueType: s.IssueType, Country: s.Country, Sector: s.Sector, Price: s.Price, PE: s.PE, YieldPct: s.Yield,
			PayoutPct: s.PayoutRatio, DivGrowth3yPct: s.DivGrowth3y, DivGrowth10yPct: s.DivGrowth10y, EPSGrowth3yPct: s.EPSGrowth3y,
			InstrumentID: s.InstrumentID}
		if s.MarketCapUSD != nil {
			v := *s.MarketCapUSD / 1e6
			row.MarketCapUSDm = &v
		}
		out = append(out, row)
	}
	key := map[string]func(ScreenStock) *float64{
		"yield": func(s ScreenStock) *float64 { return s.YieldPct }, "mcap": func(s ScreenStock) *float64 { return s.MarketCapUSDm },
		"divgrowth3y": func(s ScreenStock) *float64 { return s.DivGrowth3yPct }, "pe": func(s ScreenStock) *float64 { return s.PE },
	}[f.sort]
	if key != nil {
		asc := f.sort == "pe"
		slices.SortStableFunc(out, func(a, b ScreenStock) int {
			x, y := key(a), key(b)
			switch {
			case x == nil && y == nil:
				return 0
			case x == nil:
				return 1
			case y == nil:
				return -1
			case asc:
				return cmp.Compare(*x, *y)
			}
			return cmp.Compare(*y, *x)
		})
	}
	matched := len(out)
	if f.limit > 0 && len(out) > f.limit {
		out = out[:f.limit]
	}
	doc := map[string]any{"as_of": e.today(), "source": "lightyear public screener", "screened": len(all), "matched": matched, "rows": out}
	if f.appendRows {
		var was struct {
			Rows []ScreenStock `yaml:"rows"`
		}
		if _, err := e.readYAML(&was, "screen", "stocks.yaml"); err != nil {
			return err
		}
		merged := mergeStockRows(was.Rows, out)
		doc["rows"], doc["matched"], doc["appended"] = merged, len(merged), len(merged)-len(was.Rows)
	}
	if _, err := e.writeYAML(doc, "screen", "stocks.yaml"); err != nil {
		return err
	}
	if e.md {
		var rows [][]string
		for _, s := range out {
			rows = append(rows, []string{s.Ticker, s.Name, s.Country, s.Currency, num(s.YieldPct, 2), num(s.PayoutPct, 0),
				num(s.DivGrowth3yPct, 1), num(s.MarketCapUSDm, 0), num(s.PE, 1)})
		}
		_, err := fmt.Fprintf(e.out, "%d of %d rows match\n\n%s", matched, len(all), mdTable(
			[]string{"Ticker", "Name", "Country", "Ccy", "Yield %", "Payout %", "Div g 3y %", "Mcap $m", "P/E"}, rows))
		return err
	}
	return e.printJSON(doc)
}

func (e *env) screenFunds(ctx context.Context, cl Screener, f screenFlags) error {
	all, err := cl.Funds(ctx, nil)
	if err != nil {
		return err
	}
	ccys, mics := listSet(f.currencies), listSet(f.mic)
	var out []ScreenFund
	for _, s := range all {
		if !inSet(ccys, s.Currency) || !inSet(mics, s.MIC) || !has(s.Name, f.name) || !has(s.Manager, f.manager) ||
			!has(s.AssetType, f.assetType) || (f.class != "" && !strings.EqualFold(s.Class, f.class)) ||
			(f.issueType != "" && !strings.EqualFold(s.IssueType, f.issueType)) {
			continue
		}
		out = append(out, ScreenFund{Ticker: s.Ticker, Name: s.Name, ISIN: s.ISIN, Exchange: s.Exchange, MIC: s.MIC,
			Currency: s.Currency, Class: s.Class, IssueType: s.IssueType, AssetType: s.AssetType, Manager: s.Manager})
	}
	matched := len(out)
	if f.limit > 0 && len(out) > f.limit {
		out = out[:f.limit]
	}
	doc := map[string]any{"as_of": e.today(), "source": "lightyear public screener", "screened": len(all), "matched": matched, "rows": out,
		"note": "the public fund screener carries no yield or TER: read them with pb ly import (fund-info) or pb ep etf"}
	if _, err := e.writeYAML(doc, "screen", "funds.yaml"); err != nil {
		return err
	}
	if e.md {
		var rows [][]string
		for _, s := range out {
			rows = append(rows, []string{s.Ticker, s.Name, s.ISIN, s.Exchange, s.Currency, s.Class, s.Manager})
		}
		_, err := fmt.Fprintf(e.out, "%d of %d funds match\n\n%s", matched, len(all), mdTable(
			[]string{"Ticker", "Name", "ISIN", "Exchange", "Ccy", "Class", "Manager"}, rows))
		return err
	}
	return e.printJSON(doc)
}

// mergeStockRows appends the new rows to the pool, keeping one row per listing (instrument id, else
// ISIN and venue, else ticker and venue).
func mergeStockRows(pool, add []ScreenStock) []ScreenStock {
	key := func(r ScreenStock) string {
		switch {
		case r.InstrumentID != "":
			return "id:" + r.InstrumentID
		case r.ISIN != "":
			return "isin:" + strings.ToUpper(r.ISIN+"@"+r.MIC)
		}
		return "t:" + strings.ToUpper(r.Ticker+"@"+r.MIC)
	}
	seen := map[string]bool{}
	out := slices.Clone(pool)
	for _, r := range pool {
		seen[key(r)] = true
	}
	for _, r := range add {
		if !seen[key(r)] {
			seen[key(r)] = true
			out = append(out, r)
		}
	}
	return out
}
