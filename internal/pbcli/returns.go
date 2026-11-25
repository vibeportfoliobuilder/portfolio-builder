package pbcli

// returns.go - pb ep returns: price CAGR, total return and dividend growth over 1, 5 and 10
// years from a cheap Eulerpool series (3 requests per stock, 2 per fund, cached), for watchlist
// names (returns.yaml, read by pb derive and pb report build) and the run's screen candidates.

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/returns"
)

// DefaultBenchmark is the S&P 500 (SPY), the market the beta and the correction falls are read against.
const DefaultBenchmark = "US78462F1030"

// Benchmark is data/benchmark.yaml: the market's monthly closes for the dividend score's recession pillar.
type Benchmark struct {
	AsOf          string             `yaml:"as_of" json:"as_of"`
	Source        string             `yaml:"source" json:"source"`
	Name          string             `yaml:"name" json:"name"`
	ISIN          string             `yaml:"isin" json:"isin"`
	Listing       string             `yaml:"listing,omitempty" json:"listing,omitempty"`
	MonthlyCloses map[string]float64 `yaml:"monthly_closes" json:"monthly_closes"`
}

func epReturnsCmd(e *env) *cobra.Command {
	var watch, screen bool
	var run, bench string
	c := &cobra.Command{
		Use:   "returns <TICKER|ISIN|TICKER=ISIN ...> | --watchlist | --screen [--run D]",
		Short: "Price CAGR, total return and dividend growth over 1, 5 and 10 years (returns.yaml, screen.yaml)",
		Long: `Reads an 11-year daily price series and the whole dividend ledger per stock (3 requests:
charting/ohlcv, equity/dividends, equity/dividends-by-fy; a name fetched today by pb ep fetch is a cache
hit) and the quote history per fund (2 requests: etf/profile, etf/quotes; the vendor picks ONE listing per
ISIN, named in "listing"). Computes per name, in percent a year:
  price_cagr_pct       1y 5y 10y to the latest close
  total_return_pct     1y 5y 10y, each dividend reinvested at its ex-month close, GROSS of withholding
  dividend_growth_pct  1y 3y 5y 10y from fully paid fiscal years (as pb ep fetch)
  yield_pct            stocks: trailing year to the last ex-date over the last close; funds: the broker's
A figure that cannot be computed is absent with a one-word reason under n_a ("no history", "no dividend",
"accumulating", "no source": Eulerpool has no fund distributions, so a distributing fund's total return
and dividend growth are "no source"; an accumulating fund's total return is its price return).
Writes data/stocks/<T>/returns.yaml or data/etfs/<T>/returns.yaml; with --screen, fills each candidate's
"returns" block in data/runs/<run>/screen.yaml (stocks and funds; duplicate fund ISINs are read once).
Always refreshes data/benchmark.yaml (the S&P 500 via SPY, 1-2 requests) for pb derive's dividend score.
Examples:
  pb ep returns --watchlist
  pb ep returns --screen --run <date>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !watch && !screen && len(args) == 0 {
				return fmt.Errorf("name tickers, or pass --watchlist or --screen")
			}
			cl, err := epClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			out := map[string]any{}
			if b, err := e.fetchBenchmark(ctx, cl, bench); err != nil {
				out["benchmark_error"] = err.Error()
			} else {
				out["benchmark"] = b
			}
			if watch || len(args) > 0 {
				ts, err := e.targets(args, watch, "")
				if err != nil {
					return err
				}
				out["names"] = e.namesReturns(ctx, cl, ts)
			}
			if screen {
				r := cmp.Or(run, e.latestRun())
				res, err := e.screenReturns(ctx, cl, r)
				if err != nil {
					return err
				}
				out["screen"] = res
			}
			out["requests"] = cl.Requests()
			return e.printJSON(out)
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "every name in data/watchlist.yaml")
	c.Flags().BoolVar(&screen, "screen", false, "every candidate in the run's screen.yaml")
	c.Flags().StringVar(&run, "run", "", "run date for --screen (default: the newest run)")
	c.Flags().StringVar(&bench, "benchmark", DefaultBenchmark, "benchmark ISIN for beta (an ETF or a stock)")
	return c
}

// stockScale is 0.01 for an LSE line priced in GBP (the vendor quotes it in pence).
func stockScale(currency, mic, exchange string) float64 {
	if strings.EqualFold(currency, "GBP") && (strings.EqualFold(mic, "XLON") || strings.EqualFold(exchange, "LSE")) {
		return 0.01
	}
	return 1
}

// stockReturns computes one stock's figures from its series.
func stockReturns(ctx context.Context, cl *eulerpool.Client, isin string, scale float64, now time.Time) (returns.Figures, error) {
	s, err := cl.StockSeries(ctx, isin, scale)
	if err != nil {
		return returns.Figures{}, err
	}
	in := returns.Series{Current: s.LastClose, DividendGrowth: s.DividendCAGR, Distributions: true}
	for _, c := range s.Closes {
		in.Closes = append(in.Closes, returns.Close{Month: c.Month, Close: c.Close})
	}
	for _, p := range s.Payments {
		in.Payments = append(in.Payments, returns.Payment{ExDate: p.ExDate, Amount: p.DPS})
	}
	f := returns.Compute(in)
	f.AsOf, f.Source = s.PriceAsOf, "eulerpool charting/ohlcv + equity/dividends + equity/dividends-by-fy"
	f.YieldPct = returns.TTMYield(in.Payments, s.LastClose, now)
	f.Specials = returns.Specials(in.Payments, now)
	if s.NoCutYears > 0 || s.IncreaseYears > 0 {
		f.Streaks = map[string]int{"no_cut": s.NoCutYears, "increase": s.IncreaseYears}
	}
	if f.YieldPct == nil {
		setNA(&f, "yield_pct", returns.NoDividend)
	}
	f.Notes = append(append([]string{returns.TotalReturnNote}, f.Notes...), s.Gaps...)
	return f, nil
}

// etfReturns computes one fund's figures; yield comes from the broker's fund data (broker.yaml).
func etfReturns(ctx context.Context, cl *eulerpool.Client, isin string, line eulerpool.Line, acc bool, lyYield *float64) (returns.Figures, error) {
	s, err := cl.ETFSeriesLine(ctx, isin, line)
	if err != nil {
		return returns.Figures{}, err
	}
	in := returns.Series{Current: s.LastClose, Accumulating: acc}
	for _, c := range s.Closes {
		in.Closes = append(in.Closes, returns.Close{Month: c.Month, Close: c.Close})
	}
	f := returns.Compute(in)
	f.AsOf, f.Source = s.PriceAsOf, "eulerpool etf/quotes (no distributions in Eulerpool)"
	f.Listing = strings.TrimSpace(s.Symbol + " " + s.Currency)
	switch {
	case acc:
		z := 0.0
		f.YieldPct = &z
		f.Notes = append(f.Notes, "accumulating: income is reinvested inside the fund, so total return is the price return")
	case lyYield != nil:
		f.YieldPct = lyYield
		f.Notes = append(f.Notes, "yield: the broker's fund data (trailing)")
	default:
		setNA(&f, "yield_pct", returns.NoSource)
	}
	if line.Symbol == "" || !strings.EqualFold(s.Symbol, line.Symbol) {
		f.Notes = append(f.Notes, "price series is the vendor's listing "+f.Listing+", which may not be the line the broker trades")
	}
	f.Notes = append(f.Notes, s.Gaps...)
	return f, nil
}

func setNA(f *returns.Figures, key, why string) {
	if f.NA == nil {
		f.NA = map[string]string{}
	}
	f.NA[key] = why
}

type returnsResult struct {
	Ticker  string `json:"ticker"`
	Kind    string `json:"kind"`
	File    string `json:"file,omitempty"`
	Listing string `json:"listing,omitempty"`
	Error   string `json:"error,omitempty"`
}

// pool runs f over n jobs, four at a time.
func pool(n int, f func(i int)) {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			f(i)
		}()
	}
	wg.Wait()
}

// isFund reports a target that is a fund: the watchlist says so, or data/etfs/<T> holds its files.
func (e *env) isFund(t target) bool {
	if t.Item != nil {
		return t.Item.Kind == kindETF
	}
	for _, f := range []string{broker.File, "financial_data.yaml"} {
		if _, err := os.Stat(e.path("etfs", t.Ticker, f)); err == nil {
			return true
		}
	}
	return false
}

// fundInfo is what a broker connector stored for a fund: its yield and whether it accumulates.
func (e *env) fundInfo(ticker, class string) (acc bool, yield *float64) {
	acc = strings.EqualFold(class, "Acc")
	var ly broker.ETF
	if ok, _ := e.readYAML(&ly, "etfs", dirName(ticker), broker.File); ok && ly.Fund != nil {
		if p := strings.ToLower(ly.Fund.DistributionPolicy); p != "" {
			acc = strings.HasPrefix(p, "acc")
		}
		yield = ly.Fund.YieldPct
	}
	return acc, yield
}

func (e *env) namesReturns(ctx context.Context, cl *eulerpool.Client, ts []target) []returnsResult {
	now := e.now().UTC()
	res := make([]returnsResult, len(ts))
	pool(len(ts), func(i int) {
		t := ts[i]
		r := returnsResult{Ticker: t.Ticker, Kind: kindStock}
		var f returns.Figures
		var err error
		if e.isFund(t) {
			r.Kind = kindETF
			acc, y := e.fundInfo(t.Ticker, "")
			f, err = etfReturns(ctx, cl, t.ID, e.fundLine(t), acc, y)
			r.Listing = f.Listing
		} else {
			cur, mic, ex := "", "", ""
			if t.Item != nil {
				cur, mic, ex = t.Item.Currency, t.Item.MIC, t.Item.Exchange
			}
			f, err = stockReturns(ctx, cl, t.ID, stockScale(cur, mic, ex), now)
		}
		if err != nil {
			r.Error = err.Error()
			res[i] = r
			return
		}
		dir := "stocks"
		if r.Kind == kindETF {
			dir = "etfs"
		}
		r.File, err = e.writeYAML(f, dir, t.Ticker, "returns.yaml")
		if err != nil {
			r.Error = err.Error()
		}
		res[i] = r
	})
	return res
}

// screenReturns fills every candidate's returns block and tax fields in the run's screen.yaml.
func (e *env) screenReturns(ctx context.Context, cl *eulerpool.Client, run string) (map[string]any, error) {
	var doc ScreenDoc
	ok, err := e.readYAML(&doc, "runs", run, "screen.yaml")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no screen.yaml for run %s", run)
	}
	now := e.now().UTC()
	var mu sync.Mutex
	var errs []string
	fail := func(t string, err error) {
		mu.Lock()
		defer mu.Unlock()
		why := err.Error()
		if errors.Is(err, eulerpool.ErrNotFound) {
			why = "not covered by Eulerpool"
		}
		errs = append(errs, t+": "+why)
	}
	pool(len(doc.Stocks), func(i int) {
		c := &doc.Stocks[i]
		if c.ISIN == "" {
			return
		}
		f, err := stockReturns(ctx, cl, c.ISIN, stockScale(c.Currency, c.MIC, ""), now)
		if err != nil {
			fail(c.Ticker, err)
			if errors.Is(err, eulerpool.ErrNotFound) {
				c.Returns = notCovered()
			}
			return
		}
		f.Notes = f.Notes[1:] // the page states the total-return basis once
		if len(f.Notes) == 0 {
			f.Notes = nil
		}
		c.Returns = &f
		addStableGroups(c)
	})
	// Funds: one series per ISIN, shared by its listings.
	byISIN := map[string]*returns.Figures{}
	var isins []string
	for _, f := range doc.Funds {
		if f.ISIN != "" && byISIN[f.ISIN] == nil {
			byISIN[f.ISIN] = &returns.Figures{}
			isins = append(isins, f.ISIN)
		}
	}
	sort.Strings(isins)
	acc := map[string]bool{}
	for _, f := range doc.Funds {
		acc[f.ISIN] = strings.EqualFold(f.Class, "Acc")
	}
	pool(len(isins), func(i int) {
		isin := isins[i]
		f, err := etfReturns(ctx, cl, isin, eulerpool.Line{}, acc[isin], nil)
		if err != nil {
			fail(isin, err)
			mu.Lock()
			byISIN[isin] = nil
			mu.Unlock()
			return
		}
		f.Notes = nil
		mu.Lock()
		*byISIN[isin] = f
		mu.Unlock()
	})
	for i := range doc.Funds {
		c := &doc.Funds[i]
		if f := byISIN[c.ISIN]; f != nil {
			cp := *f
			if a, y := e.fundInfo(c.Ticker, c.Class); y != nil && !a {
				cp.YieldPct = y
				delete(cp.NA, "yield_pct")
			}
			c.Returns = &cp
		}
	}
	fillScreenTax(&doc)
	p, err := e.writeYAML(doc, "runs", run, "screen.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(errs)
	return map[string]any{"file": p, "stocks": len(doc.Stocks), "funds": len(doc.Funds), "fund_isins": len(isins), "errors": errs}, nil
}

// addStableGroups adds Stable Payer (stable_payer_years with no cut) and Stable Grower
// (stable_grower_years of increases) from the ledger's streaks, which the screener rows lack; a
// group in large_cap_only_groups needs a mega or large cap.
func addStableGroups(c *ScreenCandidate) {
	if c.Returns == nil || c.Returns.Streaks == nil {
		return
	}
	large := c.CapBand == rules.Mega || c.CapBand == rules.Large
	add := func(g string) {
		if !large && slices.ContainsFunc(pack.Active().Strategy.LargeCapOnlyGroups, func(x string) bool { return strings.EqualFold(strings.TrimSpace(x), g) }) {
			return
		}
		if !slices.Contains(c.Groups, g) {
			c.Groups = append(c.Groups, g)
		}
		if c.Group == "" || c.Group == "none" || c.Group == rules.Growth {
			c.Group = g
		}
	}
	st := pack.Active().Strategy
	if st.StableGrowerYears != nil && c.Returns.Streaks["increase"] >= *st.StableGrowerYears {
		add(rules.StableGrower)
	}
	if st.StablePayerYears != nil && c.Returns.Streaks["no_cut"] >= *st.StablePayerYears {
		add(rules.StablePayer)
	}
}

// fillScreenTax states each candidate's withholding: a stock's payer country, tier and broker
// rate; a fund's domicile and, where one applies, its tier.
func fillScreenTax(doc *ScreenDoc) {
	for i := range doc.Stocks {
		c := &doc.Stocks[i]
		if c.Withholding == nil {
			c.Withholding = rules.WithholdingFor(c.Country, c.ISIN, rules.IsREIT(c.Sector, ""))
		}
	}
	for i := range doc.Funds {
		c := &doc.Funds[i]
		c.Domicile = cmp.Or(c.Domicile, rules.FundDomicile(c.ISIN))
		if c.Withholding == nil {
			c.Withholding = rules.FundWithholding(c.ISIN)
		}
	}
}

// fetchBenchmark writes data/benchmark.yaml from the fund quotes, else the stock price series.
func (e *env) fetchBenchmark(ctx context.Context, cl *eulerpool.Client, isin string) (map[string]any, error) {
	b := Benchmark{AsOf: e.today(), Source: "eulerpool", Name: "S&P 500 (SPY)", ISIN: isin, MonthlyCloses: map[string]float64{}}
	if isin != DefaultBenchmark {
		b.Name = isin
	}
	if s, err := cl.ETFSeries(ctx, isin); err == nil {
		b.Listing = strings.TrimSpace(s.Symbol + " " + s.Currency)
		for _, c := range s.Closes {
			b.MonthlyCloses[c.Month] = c.Close
		}
	} else if s, err2 := cl.StockSeries(ctx, isin, 1); err2 == nil {
		for _, c := range s.Closes {
			b.MonthlyCloses[c.Month] = c.Close
		}
	} else {
		return nil, fmt.Errorf("benchmark %s: etf quotes: %v; prices: %v", isin, err, err2)
	}
	p, err := e.writeYAML(b, "benchmark.yaml")
	if err != nil {
		return nil, err
	}
	return map[string]any{"file": p, "listing": b.Listing, "months": len(b.MonthlyCloses)}, nil
}

// benchCloses reads data/benchmark.yaml as a series; nil when absent.
func (e *env) benchCloses() []returns.Close {
	var b Benchmark
	if ok, _ := e.readYAML(&b, "benchmark.yaml"); !ok {
		return nil
	}
	return closesOf(b.MonthlyCloses)
}

func closesOf(m map[string]float64) []returns.Close {
	out := make([]returns.Close, 0, len(m))
	for k, v := range m {
		out = append(out, returns.Close{Month: k, Close: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out
}

// NotCovered is the reason for a name the vendor does not carry (a 404).
const NotCovered = "not covered"

// notCovered is a returns block whose every figure is absent because Eulerpool lacks the name.
func notCovered() *returns.Figures {
	f := &returns.Figures{NA: map[string]string{"yield_pct": NotCovered}, Notes: []string{"Eulerpool does not cover this listing (http 404)"}}
	for _, m := range []string{"price_cagr_pct", "total_return_pct", "dividend_growth_pct"} {
		for _, y := range returns.Horizons {
			f.NA[m+"."+returns.Key(y)] = NotCovered
		}
	}
	return f
}
