package pbcli

// watchadd.go - pb watchlist add: one more name after gate 1, taken from the run's screen.yaml, else the screener pools.

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func watchlistAddCmd(e *env) *cobra.Command {
	var run string
	c := &cobra.Command{
		Use:   "add <TICKER[@MIC] ...>",
		Short: "Add names to data/watchlist.yaml from the run's screen.yaml or the screener pools",
		Long: `Looks each name up in the run's screen.yaml (stocks, then funds), else in data/screen/stocks.yaml and
funds.yaml, and appends it to data/watchlist.yaml with its ISIN, venue, currency, country and the screener's
instrument id. TICKER@MIC picks one listing when the ticker has several (IUSA@XAMS); without it the first
row in an income currency is taken. A name already on the watchlist is left as it is. Then run the data
steps for it (your broker connector's sync, pb ep fetch|etf, pb derive).
Examples:
  pb watchlist add SCHD --run <date>
  pb watchlist add IUSA@XAMS KO`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := e.readWatchlist()
			if err != nil {
				return err
			}
			var doc ScreenDoc
			if _, err := e.readYAML(&doc, "runs", cmp.Or(run, e.latestRun()), "screen.yaml"); err != nil {
				return err
			}
			var st struct {
				Rows []ScreenStock `yaml:"rows"`
			}
			var fu struct {
				Rows []ScreenFund `yaml:"rows"`
			}
			if _, err := e.readYAML(&st, "screen", "stocks.yaml"); err != nil {
				return err
			}
			if _, err := e.readYAML(&fu, "screen", "funds.yaml"); err != nil {
				return err
			}
			var added, kept, unknown []string
			for _, a := range args {
				t, mic, _ := strings.Cut(strings.ToUpper(strings.TrimSpace(a)), "@")
				if onWatchlist(w, t, mic) {
					kept = append(kept, t)
					continue
				}
				it, ok := findWatchItem(doc, st.Rows, fu.Rows, t, mic)
				if !ok {
					unknown = append(unknown, a)
					continue
				}
				w.Items = append(w.Items, it)
				added = append(added, it.Ticker+" ("+it.Kind+", "+cmp.Or(it.MIC, it.Exchange)+" "+it.Currency+")")
			}
			if len(unknown) > 0 {
				return fmt.Errorf("not in the run's screen.yaml or data/screen/*.yaml: %s (run pb ly screen ... --append for it)", strings.Join(unknown, ", "))
			}
			if len(added) > 0 {
				w.AsOf = e.today()
				if _, err := e.writeYAML(w, "watchlist.yaml"); err != nil {
					return err
				}
			}
			return e.printJSON(map[string]any{"file": e.path("watchlist.yaml"), "added": added, "already_on_watchlist": kept, "items": len(w.Items)})
		},
	}
	c.Flags().StringVar(&run, "run", "", "run whose screen.yaml to look in (default: the newest run)")
	return c
}

func onWatchlist(w Watchlist, t, mic string) bool {
	for _, it := range w.Items {
		if strings.EqualFold(it.Ticker, t) && (mic == "" || strings.EqualFold(it.MIC, mic) || strings.EqualFold(it.Exchange, mic)) {
			return true
		}
	}
	return false
}

// findWatchItem looks a ticker (and venue) up in the run's candidates, then the screener pools.
func findWatchItem(doc ScreenDoc, stocks []ScreenStock, funds []ScreenFund, t, mic string) (WatchItem, bool) {
	match := func(ticker, m, ccy string) bool {
		return strings.EqualFold(ticker, t) && (mic == "" && incomeCurrency(ccy) || strings.EqualFold(m, mic))
	}
	for _, c := range doc.Stocks {
		if match(c.Ticker, c.MIC, c.Currency) {
			return WatchItem{Ticker: c.Ticker, Name: c.Name, Kind: kindStock, ISIN: c.ISIN, MIC: c.MIC, Currency: c.Currency,
				Country: c.Country, Sector: c.Sector, InstrumentID: c.InstrumentID}, true
		}
	}
	for _, c := range doc.Funds {
		if match(c.Ticker, cmp.Or(c.MIC, c.Exchange), c.Currency) {
			return WatchItem{Ticker: c.Ticker, Name: c.Name, Kind: kindETF, ISIN: c.ISIN, Exchange: c.Exchange, MIC: c.MIC,
				Currency: c.Currency, InstrumentID: c.InstrumentID}, true
		}
	}
	for _, s := range stocks {
		if match(s.Ticker, s.MIC, s.Currency) {
			return WatchItem{Ticker: s.Ticker, Name: s.Name, Kind: kindOf(s.IssueType), ISIN: s.ISIN, IssueType: s.IssueType, MIC: s.MIC,
				Currency: s.Currency, Country: countryCode(s.Country), Sector: s.Sector, InstrumentID: s.InstrumentID}, true
		}
	}
	for _, f := range funds {
		if match(f.Ticker, cmp.Or(f.MIC, f.Exchange), f.Currency) {
			return WatchItem{Ticker: f.Ticker, Name: f.Name, Kind: kindETF, ISIN: f.ISIN, IssueType: f.IssueType, Exchange: f.Exchange,
				MIC: f.MIC, Currency: f.Currency, InstrumentID: f.InstrumentID}, true
		}
	}
	return WatchItem{}, false
}
