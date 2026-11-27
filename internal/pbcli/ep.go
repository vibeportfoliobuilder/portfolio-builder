package pbcli

// ep.go - pb ep fetch | etf | macro | news: Eulerpool dossiers for the watchlist.

import (
	"errors"
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

func epCmd(e *env) *cobra.Command {
	c := &cobra.Command{
		Use:   "ep",
		Short: "Eulerpool: financials, dividends, ETFs, macro and US news (needs EULERPOOL_API_KEY)",
		Long: `Eulerpool supplies statements, dividends, ratios, estimates, ownership, insiders, prices,
ETF profiles and holdings, and macro data. Raw responses are cached outside the repository at
os.UserCacheDir()/pb/eulerpool (200 MB cap, 30-day prune). Pass ISINs where you can: the
vendor's tickers carry suffixes (VOD.L, VOW3.DE). A name may be given as TICKER=ISIN.`,
	}
	c.AddCommand(epFetchCmd(e), epETFCmd(e), epMacroCmd(e), epNewsCmd(e), epReturnsCmd(e))
	return c
}

// epClient builds the Eulerpool client with its disk cache.
func epClient() (*eulerpool.Client, error) {
	c, err := eulerpool.NewClient()
	if err != nil {
		return nil, err
	}
	if d, err := eulerpool.DefaultCacheDir(); err == nil {
		c.Cache = eulerpool.NewDiskCache(d)
	}
	return c, nil
}

// fetchResult is one name's line in a fetch report.
type fetchResult struct {
	Ticker string `yaml:"ticker" json:"ticker"`
	ID     string `yaml:"id" json:"id"`
	File   string `yaml:"file,omitempty" json:"file,omitempty"`
	Gaps   int    `yaml:"data_gaps,omitempty" json:"data_gaps,omitempty"`
	Error  string `yaml:"error,omitempty" json:"error,omitempty"`
}

// parallel runs f over targets, four at a time, keeping the input order.
func parallel(ts []target, f func(target) fetchResult) []fetchResult {
	out := make([]fetchResult, len(ts))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, t := range ts {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = f(t)
		}()
	}
	wg.Wait()
	return out
}

func epFetchCmd(e *env) *cobra.Command {
	var watch bool
	c := &cobra.Command{
		Use:   "fetch <TICKER|ISIN|TICKER=ISIN ...> | --watchlist",
		Short: "Fetch the Eulerpool dossier of stocks into data/stocks/<T>/financial_data.yaml",
		Long: `About 20 requests per name when cold. Writes data/stocks/<T>/financial_data.yaml (20–35 KB):
identity, data_gaps, price (CAGRs, monthly closes), ratios (payout, Debt/FCF, PEGY), analyst targets and
estimates, TTM/annual/quarterly statements, dividend_history (streaks, CAGRs, by fiscal year), shares,
insider transactions. A 404 (not covered, e.g. a preferred share) is reported and the run goes on.
Examples:
  pb ep fetch KO VOD=GB00BH4HKS39
  pb ep fetch --watchlist`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ts, err := e.targets(args, watch, kindStock)
			if err != nil {
				return err
			}
			cl, err := epClient()
			if err != nil {
				return err
			}
			res := parallel(ts, func(t target) fetchResult {
				r := fetchResult{Ticker: t.Ticker, ID: t.ID}
				d, err := cl.Fetch(cmd.Context(), t.ID)
				if err != nil {
					r.Error = err.Error()
					if errors.Is(err, eulerpool.ErrNotFound) {
						r.Error = "not covered by Eulerpool: " + err.Error()
					}
					return r
				}
				r.Gaps = len(d.DataGaps)
				r.File, err = e.writeYAML(d, "stocks", t.Ticker, "financial_data.yaml")
				if err != nil {
					r.Error = err.Error()
				}
				return r
			})
			if err := e.writeVendorGaps(res); err != nil {
				return err
			}
			return e.printJSON(map[string]any{"results": res, "requests": cl.Requests()})
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "every stock in data/watchlist.yaml")
	return c
}

func epETFCmd(e *env) *cobra.Command {
	var watch bool
	c := &cobra.Command{
		Use:   "etf <ISIN|TICKER=ISIN ...> | --watchlist",
		Short: "Fetch Eulerpool ETF data into data/etfs/<T>/financial_data.yaml",
		Long: `5 requests per fund: profile (TER, AUM, NAV), quotes, top holdings, countries, sectors.
Eulerpool has NO ETF distributions: the yield comes from the broker's fund data (broker.yaml).
An ISIN alone resolves to one listing of the vendor's choosing (a USD fund can come back as its EUR line), so the
line the broker trades (broker.yaml's symbol and venue, e.g. VALU.L) is asked for first (one more
request); data_gaps names the line used, holes in the quote series and a price far from the NAV.
Examples:
  pb ep etf CSPX=IE00B5BMR087
  pb ep etf --watchlist`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ts, err := e.targets(args, watch, kindETF)
			if err != nil {
				return err
			}
			cl, err := epClient()
			if err != nil {
				return err
			}
			res := parallel(ts, func(t target) fetchResult {
				r := fetchResult{Ticker: t.Ticker, ID: t.ID}
				d, err := cl.ETFLine(cmd.Context(), t.ID, e.fundLine(t))
				if err != nil {
					r.Error = err.Error()
					return r
				}
				r.Gaps = len(d.DataGaps)
				r.File, err = e.writeYAML(d, "etfs", t.Ticker, "financial_data.yaml")
				if err != nil {
					r.Error = err.Error()
				}
				return r
			})
			return e.printJSON(map[string]any{"results": res, "requests": cl.Requests()})
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "every ETF in data/watchlist.yaml")
	return c
}

func epMacroCmd(e *env) *cobra.Command {
	var watch bool
	var countries string
	var back, ahead int
	c := &cobra.Command{
		Use:   "macro [--watchlist] [--countries DE,US,...]",
		Short: "Fetch macro data for the watchlist's countries into data/macro.yaml",
		Long: `7–9 requests: policy rates, US rates, FX against the home currency (1m and 1y change), CPI, the macro calendar
(60 days back, 28 ahead) and country risk. The country pack's macro_areas are always included; --watchlist adds the
countries of the watched stocks (from financial_data.yaml or watchlist.yaml).
pb derive reads the FX block to convert market caps into USD, so run this before derive.
Example:
  pb ep macro --watchlist`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cs := listSet(countries)
			if watch {
				w, err := e.readWatchlist()
				if err != nil {
					return err
				}
				for _, it := range w.Items {
					c := it.Country
					var d eulerpool.Dossier
					if ok, _ := e.readYAML(&d, "stocks", dirName(it.Ticker), "financial_data.yaml"); ok && d.Identity.Country != "" {
						c = d.Identity.Country
					}
					if c = strings.ToUpper(c); c != "" && !slices.Contains(cs, c) {
						cs = append(cs, c)
					}
				}
			}
			cl, err := epClient()
			if err != nil {
				return err
			}
			m, err := cl.Macro(cmd.Context(), cs, eulerpool.MacroOptions{LookbackDays: back, AheadDays: ahead, Home: pack.Home(), BaseAreas: pack.Active().Country.MacroAreas, Currencies: pack.Active().Strategy.IncomeCurrencies})
			if err != nil {
				return err
			}
			p, err := e.writeYAML(m, "macro.yaml")
			if err != nil {
				return err
			}
			return e.printJSON(map[string]any{"file": p, "countries": m.Countries, "fx": m.FX, "data_gaps": m.DataGaps, "requests": cl.Requests()})
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "add the watchlist's countries")
	c.Flags().StringVar(&countries, "countries", "", "comma-separated alpha-2 countries to add")
	c.Flags().IntVar(&back, "lookback-days", 0, "calendar days back (default 60)")
	c.Flags().IntVar(&ahead, "ahead-days", 0, "calendar days ahead (default 28)")
	return c
}

func epNewsCmd(e *env) *cobra.Command {
	var n int
	var market bool
	c := &cobra.Command{
		Use:   "news <TICKER|ISIN> | --market",
		Short: "Eulerpool company news (fresh for US names only) into data/stocks/<T>/news_eulerpool.yaml",
		Long: `Reads research/news for one name. US news is fresh; non-US feeds froze in Feb 2026, so use
pb news search (the broker's feed, when a connector supplies one) for non-US names. --market reads the general market feed into data/market_news.yaml.
Examples:
  pb ep news KO
  pb ep news --market`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := epClient()
			if err != nil {
				return err
			}
			if market {
				as, err := cl.MarketNews(cmd.Context(), n)
				if err != nil {
					return err
				}
				if _, err := e.writeYAML(map[string]any{"as_of": e.today(), "source": "eulerpool", "articles": as}, "market_news.yaml"); err != nil {
					return err
				}
				return e.printJSON(as)
			}
			ts, err := e.targets(args, false, "")
			if err != nil {
				return err
			}
			if len(ts) != 1 {
				return fmt.Errorf("one name at a time")
			}
			as, err := cl.News(cmd.Context(), ts[0].ID, n)
			if err != nil {
				return err
			}
			if _, err := e.writeYAML(map[string]any{"as_of": e.today(), "source": "eulerpool", "articles": as}, "stocks", ts[0].Ticker, "news_eulerpool.yaml"); err != nil {
				return err
			}
			return e.printJSON(as)
		},
	}
	c.Flags().IntVar(&n, "n", 20, "how many articles")
	c.Flags().BoolVar(&market, "market", false, "the general market feed instead of one name")
	return c
}

// writeVendorGaps keeps the full text of the gaps true of every Eulerpool dossier once, in
// data/vendor_gaps.yaml, after a fetch that wrote at least one dossier (each names it in one line).
func (e *env) writeVendorGaps(res []fetchResult) error {
	for _, r := range res {
		if r.File != "" && r.Error == "" {
			_, err := e.writeYAML(map[string]any{"eulerpool": eulerpool.VendorGaps()}, eulerpool.VendorGapsFile)
			return err
		}
	}
	return nil
}
