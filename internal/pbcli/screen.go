package pbcli

// screen.go - pb screen draft (the screener rows ranked by the profile's rules into a run's
// screen.yaml, which /screen-analyst edits) and pb watchlist from-picks (the Artifact's Screen-page
// picks back into data/watchlist.yaml).

import (
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/returns"
)

// ScreenDoc is data/runs/<date>/screen.yaml: the candidates the Screen page shows.
type ScreenDoc struct {
	AsOf   string            `yaml:"as_of" json:"as_of"`
	Source string            `yaml:"source" json:"source"`
	Notes  []string          `yaml:"notes,omitempty" json:"notes,omitempty"`
	Stocks []ScreenCandidate `yaml:"stocks" json:"stocks"`
	Funds  []FundCandidate   `yaml:"funds" json:"funds"`
}

// ScreenCandidate is one proposed stock. Reason is /screen-analyst's one line.
type ScreenCandidate struct {
	Rank     int    `yaml:"rank" json:"rank"`
	Ticker   string `yaml:"ticker" json:"ticker"`
	Name     string `yaml:"name" json:"name"`
	ISIN     string `yaml:"isin,omitempty" json:"isin,omitempty"`
	MIC      string `yaml:"mic,omitempty" json:"mic,omitempty"`
	Country  string `yaml:"country,omitempty" json:"country,omitempty"`
	Currency string `yaml:"currency,omitempty" json:"currency,omitempty"`
	Sector   string `yaml:"sector,omitempty" json:"sector,omitempty"`
	// Industry is finer than the screener's sector: Eulerpool's profile, else the broker's own screener (pb screen enrich --broker).
	Industry       string   `yaml:"industry,omitempty" json:"industry,omitempty"`
	Group          string   `yaml:"group" json:"group"`
	Groups         []string `yaml:"groups,omitempty" json:"groups,omitempty"`
	YieldPct       *float64 `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	NetYieldPct    *float64 `yaml:"net_yield_pct,omitempty" json:"net_yield_pct,omitempty"`
	WithholdingPct *float64 `yaml:"withholding_pct,omitempty" json:"withholding_pct,omitempty"`
	Growth3yPct    *float64 `yaml:"growth_3y_pct,omitempty" json:"growth_3y_pct,omitempty"`
	Growth10yPct   *float64 `yaml:"growth_10y_pct,omitempty" json:"growth_10y_pct,omitempty"`
	PayoutPct      *float64 `yaml:"payout_pct,omitempty" json:"payout_pct,omitempty"`
	PE             *float64 `yaml:"pe,omitempty" json:"pe,omitempty"`
	CapBand        string   `yaml:"cap_band,omitempty" json:"cap_band,omitempty"`
	MarketCapUSDm  *float64 `yaml:"market_cap_usd_m,omitempty" json:"market_cap_usd_m,omitempty"`
	Flags          []string `yaml:"flags,omitempty" json:"flags,omitempty"`
	InstrumentID   string   `yaml:"instrument_id,omitempty" json:"instrument_id,omitempty"`
	Reason         string   `yaml:"reason" json:"reason"`
	// Withholding is the payer country, tier and broker rate; Returns comes from pb ep returns --screen.
	Withholding *rules.Withholding `yaml:"withholding,omitempty" json:"withholding,omitempty"`
	Returns     *returns.Figures   `yaml:"returns,omitempty" json:"returns,omitempty"`
}

// FundCandidate is one proposed fund. Bucket is a first guess from the name and class.
type FundCandidate struct {
	Rank         int    `yaml:"rank" json:"rank"`
	Ticker       string `yaml:"ticker" json:"ticker"`
	Name         string `yaml:"name" json:"name"`
	ISIN         string `yaml:"isin,omitempty" json:"isin,omitempty"`
	Exchange     string `yaml:"exchange,omitempty" json:"exchange,omitempty"`
	MIC          string `yaml:"mic,omitempty" json:"mic,omitempty"`
	Currency     string `yaml:"currency,omitempty" json:"currency,omitempty"`
	Class        string `yaml:"class,omitempty" json:"class,omitempty"`
	AssetType    string `yaml:"asset_type,omitempty" json:"asset_type,omitempty"`
	Manager      string `yaml:"manager,omitempty" json:"manager,omitempty"`
	Bucket       string `yaml:"bucket" json:"bucket"`
	InstrumentID string `yaml:"instrument_id,omitempty" json:"instrument_id,omitempty"`
	Reason       string `yaml:"reason" json:"reason"`
	// Domicile is the ISIN's country; Withholding applies to a US fund (30%) or an IE/LU UCITS (0%).
	Domicile    string             `yaml:"domicile,omitempty" json:"domicile,omitempty"`
	Withholding *rules.Withholding `yaml:"withholding,omitempty" json:"withholding,omitempty"`
	Returns     *returns.Figures   `yaml:"returns,omitempty" json:"returns,omitempty"`
	// Facts are what the public screener lacks (pb screen enrich): yield, TER, policy and AUM from
	// the broker's own screener, TER, AUM and the holdings of 0.5%+ from Eulerpool.
	Facts *FundFacts `yaml:"facts,omitempty" json:"facts,omitempty"`
	// Favourites is computed by pb report build (the investor's names inside), never stored.
	Favourites []Favourite `yaml:"-" json:"favourites"`
}

// FundFacts is one fund's facts and where each came from.
type FundFacts struct {
	YieldPct           *float64                `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	TERPct             *float64                `yaml:"ter_pct,omitempty" json:"ter_pct,omitempty"`
	AUMMillions        *float64                `yaml:"aum_millions,omitempty" json:"aum_millions,omitempty"`
	DistributionPolicy string                  `yaml:"distribution_policy,omitempty" json:"distribution_policy,omitempty"`
	Benchmark          string                  `yaml:"benchmark,omitempty" json:"benchmark,omitempty"`
	Holdings           []eulerpool.FundHolding `yaml:"holdings_over_half_pct,omitempty" json:"holdings_over_half_pct,omitempty"`
	Sources            []string                `yaml:"sources,omitempty" json:"sources,omitempty"`
}

// excluded is why the strategy excludes a ticker ("" when it does not).
func excluded(ticker string) (string, bool) {
	for t, why := range pack.Active().Strategy.ExcludedTickers {
		if strings.EqualFold(t, ticker) {
			return cmpOrStr(why, "excluded by your strategy"), true
		}
	}
	return "", false
}

func cmpOrStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// incomeCurrency is a currency the investor takes income in: the strategy's income currencies (any
// currency when it names none) or the home currency.
func incomeCurrency(c string) bool {
	c = strings.ToUpper(c)
	allowed := pack.Active().Strategy.IncomeCurrencies
	if len(allowed) == 0 || c == pack.Home() {
		return true
	}
	for _, a := range allowed {
		if strings.EqualFold(a, c) {
			return true
		}
	}
	return false
}

// groupOrder ranks the profile's groups for the draft; the analyst reorders freely.
var groupOrder = map[string]int{rules.GOAT: 0, rules.Yielder: 1, rules.Grower: 2, rules.StableGrower: 3,
	rules.StablePayer: 4, rules.Growth: 5}

func screenCmd(e *env) *cobra.Command {
	c := &cobra.Command{Use: "screen", Short: "The run's screen: candidates for the Artifact's Screen page"}
	var run, include string
	var nStocks, nFunds int
	draft := &cobra.Command{
		Use:   "draft",
		Short: "Rank data/screen/{stocks,funds}.yaml by the profile's rules into data/runs/<date>/screen.yaml",
		Long: `Reads the rows pb ly screen saved and drops what YOUR strategy excludes (data/profile/strategy.yaml):
excluded_tickers; stocks and funds paying in a currency outside income_currencies (the home currency is
always allowed; with no income_currencies every currency is); funds whose name or asset type contains one
of excluded_fund_words. It applies the group, cap-band and withholding rules to each stock, flags a payout
above max_payout_pct, and ranks: group (GOAT, Yielder, Grower, Stable Grower, Stable Payer, Growth) taken
round-robin so every group is represented; within a dividend group by net yield plus half the 3-year
dividend growth, within Growth by market cap (no dividend to rank on). No starting-yield band is applied.
Funds get a first bucket guess (income, dividend_growth, all_cap, growth). Every reason is left empty for
/screen-analyst to write.
--include keeps the named tickers whatever the cut (first in the list, flagged "included"), and the
strategy's liked_tickers are always included: names a yield-floored pool would drop. A name must be in the
pool: add it with pb ly screen stocks ... --append.
Examples:
  pb screen draft --run <date> --stocks 100 --funds 30
  pb screen draft --run <date> --include KO,JNJ`,
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := e.screenDraft(nStocks, nFunds, append(listSet(include), pack.Active().Strategy.LikedTickers...)...)
			if err != nil {
				return err
			}
			p, err := e.writeYAML(doc, "runs", cmp.Or(run, e.today()), "screen.yaml")
			if err != nil {
				return err
			}
			return e.printJSON(map[string]any{"file": p, "stocks": len(doc.Stocks), "funds": len(doc.Funds), "notes": doc.Notes})
		},
	}
	draft.Flags().StringVar(&run, "run", "", "run date (default today)")
	draft.Flags().IntVar(&nStocks, "stocks", 100, "stock candidates to keep")
	draft.Flags().IntVar(&nFunds, "funds", 30, "fund candidates to keep")
	draft.Flags().StringVar(&include, "include", "", "comma-separated tickers kept whatever the cut (they must be in data/screen/stocks.yaml)")
	c.AddCommand(draft, screenEnrichCmd(e))
	return c
}

// IncludedFlag marks a stock kept by pb screen draft --include.
const IncludedFlag = "included by name (--include)"

func (e *env) screenDraft(nStocks, nFunds int, include ...string) (ScreenDoc, error) {
	doc := ScreenDoc{AsOf: e.today(), Source: "pb screen draft over the public screener"}
	var st struct {
		Rows []ScreenStock `yaml:"rows"`
	}
	var fu struct {
		Rows []ScreenFund `yaml:"rows"`
	}
	okS, err := e.readYAML(&st, "screen", "stocks.yaml")
	if err != nil {
		return doc, err
	}
	okF, err := e.readYAML(&fu, "screen", "funds.yaml")
	if err != nil {
		return doc, err
	}
	if !okS && !okF {
		return doc, fmt.Errorf("no screen rows: run pb ly screen stocks and pb ly screen funds first")
	}
	type ranked struct {
		c     ScreenCandidate
		order int
		score float64
	}
	var rs []ranked
	dropped := map[string]int{}
	for _, s := range st.Rows {
		if why, bad := excluded(s.Ticker); bad {
			dropped[why]++
			continue
		}
		if !incomeCurrency(s.Currency) {
			dropped["currency "+s.Currency]++
			continue
		}
		in := rules.Input{Ticker: s.Ticker, ISIN: s.ISIN, Country: countryCode(s.Country), Sector: s.Sector,
			YieldPct: s.YieldPct, Growth3y: s.DivGrowth3yPct, Growth10y: s.DivGrowth10yPct, PayoutPct: s.PayoutPct,
			MarketCapUSDm: s.MarketCapUSDm}
		v := rules.Evaluate(in)
		c := ScreenCandidate{Ticker: s.Ticker, Name: s.Name, ISIN: s.ISIN, MIC: s.MIC, Country: countryCode(s.Country),
			Currency: s.Currency, Sector: s.Sector, Group: v.Group, Groups: v.Groups, YieldPct: s.YieldPct,
			NetYieldPct: v.NetYieldPct, Growth3yPct: s.DivGrowth3yPct, Growth10yPct: s.DivGrowth10yPct,
			PayoutPct: s.PayoutPct, PE: s.PE, CapBand: v.CapBand, MarketCapUSDm: s.MarketCapUSDm, InstrumentID: s.InstrumentID}
		if v.Withholding != nil {
			w := v.Withholding.RatePct
			c.WithholdingPct = &w
		}
		st := pack.Active().Strategy
		limit := st.MaxPayoutPct
		if v.REIT {
			limit = st.MaxREITPayoutPct
		}
		if limit != nil && s.PayoutPct != nil && *s.PayoutPct >= *limit {
			c.Flags = append(c.Flags, fmt.Sprintf("payout %.0f%% >= %.0f%%", *s.PayoutPct, *limit))
		}
		if cap, ok := st.CurrencyIncomeCapPct[strings.ToUpper(s.Currency)]; ok {
			c.Flags = append(c.Flags, fmt.Sprintf("%s income (portfolio cap %g%%)", strings.ToUpper(s.Currency), cap))
		}
		order, ok := groupOrder[v.Group]
		if !ok {
			order = len(groupOrder)
		}
		score := 0.0
		if v.NetYieldPct != nil {
			score += *v.NetYieldPct
		}
		if s.DivGrowth3yPct != nil {
			score += 0.5 * math.Max(-10, math.Min(25, *s.DivGrowth3yPct))
		}
		if len(c.Flags) > 0 && strings.HasPrefix(c.Flags[0], "payout") {
			score -= 5
		}
		if v.Group == rules.Growth {
			// A growth name is held for price growth: rank it by size, not by its token yield.
			score = 0
			if s.MarketCapUSDm != nil && *s.MarketCapUSDm > 0 {
				score = math.Log10(*s.MarketCapUSDm)
			}
		}
		rs = append(rs, ranked{c, order, score})
	}
	slices.SortStableFunc(rs, func(a, b ranked) int {
		if a.order != b.order {
			return a.order - b.order
		}
		return cmp.Compare(b.score, a.score)
	})
	// Round-robin over the groups in profile order, so a cut at N keeps every group represented;
	// names in no group come last.
	byGroup := map[int][]ranked{}
	maxOrder := 0
	for _, r := range rs {
		byGroup[r.order] = append(byGroup[r.order], r)
		maxOrder = max(maxOrder, r.order)
	}
	var picked []ranked
	for progressed := true; progressed; {
		progressed = false
		for o := 0; o < len(groupOrder); o++ {
			if len(byGroup[o]) > 0 {
				picked = append(picked, byGroup[o][0])
				byGroup[o] = byGroup[o][1:]
				progressed = true
			}
		}
	}
	for o := len(groupOrder); o <= maxOrder; o++ {
		picked = append(picked, byGroup[o]...)
	}
	// Named names first, whatever the cut; then the round-robin fills up to nStocks.
	want := map[string]bool{}
	for _, t := range include {
		want[strings.ToUpper(t)] = true
	}
	found := map[string]bool{}
	var front, rest []ranked
	for _, r := range picked {
		if t := strings.ToUpper(r.c.Ticker); want[t] {
			found[t] = true
			r.c.Flags = append(r.c.Flags, IncludedFlag)
			front = append(front, r)
			continue
		}
		rest = append(rest, r)
	}
	for _, t := range include {
		if !found[strings.ToUpper(t)] {
			doc.Notes = append(doc.Notes, fmt.Sprintf("include %s: not in data/screen/stocks.yaml or dropped by the profile; add its listing with pb ly screen stocks ... --append", strings.ToUpper(t)))
		}
	}
	for _, r := range append(front, rest...) {
		if nStocks > 0 && len(doc.Stocks) >= max(nStocks, len(front)) {
			break
		}
		r.c.Rank = len(doc.Stocks) + 1
		doc.Stocks = append(doc.Stocks, r.c)
	}
	for _, f := range fu.Rows {
		at := strings.ToLower(f.AssetType + " " + f.Name)
		if w := fundWordExcluded(at); w != "" {
			dropped["fund named "+w+" (excluded_fund_words)"]++
			continue
		}
		if !incomeCurrency(f.Currency) {
			dropped["fund currency "+f.Currency]++
			continue
		}
		c := FundCandidate{Ticker: f.Ticker, Name: f.Name, ISIN: f.ISIN, Exchange: f.Exchange, MIC: f.MIC, Currency: f.Currency,
			Class: f.Class, AssetType: f.AssetType, Manager: f.Manager, Bucket: fundBucket(f), InstrumentID: f.InstrumentID}
		if nFunds > 0 && len(doc.Funds) >= nFunds {
			break
		}
		c.Rank = len(doc.Funds) + 1
		doc.Funds = append(doc.Funds, c)
	}
	fillScreenTax(&doc)
	keys := make([]string, 0, len(dropped))
	for k := range dropped {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		doc.Notes = append(doc.Notes, fmt.Sprintf("dropped %d: %s", dropped[k], k))
	}
	return doc, nil
}

// fundBucket guesses the fund bucket from the fund's name and class.
func fundBucket(f ScreenFund) string {
	n := strings.ToLower(f.Name)
	switch {
	case strings.Contains(n, "covered call") || strings.Contains(n, "premium") || strings.Contains(n, "option") || strings.Contains(n, "income"):
		return "income"
	case strings.Contains(n, "dividend") || strings.Contains(n, "aristocrat") || strings.Contains(n, "yield"):
		return "dividend_growth"
	case strings.Contains(n, "semiconductor") || strings.Contains(n, "technology") || strings.Contains(n, "nasdaq") || strings.Contains(n, "tech"):
		return "growth"
	case strings.Contains(n, "world") || strings.Contains(n, "all-world") || strings.Contains(n, "all world") || strings.Contains(n, "acwi"):
		return "all_cap"
	}
	return "other"
}

// Gate 1 takes 25 to 35 picked stocks; the run never trims a pick.
const (
	MinPickedStocks = 25
	MaxPickedStocks = 35
)

// Picks is what the Artifact's Screen page saves (collection picks, one document per run).
// StockKeys and FundKeys name each picked row exactly (the screener repeats a ticker on several
// venues and companies); a picks file without them is matched by ticker.
type Picks struct {
	Run       string    `yaml:"run,omitempty" json:"run,omitempty"`
	Stocks    []string  `yaml:"stocks" json:"stocks"`
	Funds     []string  `yaml:"funds" json:"funds"`
	StockKeys []PickKey `yaml:"stock_keys,omitempty" json:"stock_keys,omitempty"`
	FundKeys  []PickKey `yaml:"fund_keys,omitempty" json:"fund_keys,omitempty"`
}

// PickKey is one picked screen row: the screener's instrument id where the screener gave one, else ISIN and venue.
type PickKey struct {
	Ticker       string `yaml:"ticker" json:"ticker"`
	InstrumentID string `yaml:"instrument_id,omitempty" json:"instrument_id,omitempty"`
	ISIN         string `yaml:"isin,omitempty" json:"isin,omitempty"`
	MIC          string `yaml:"mic,omitempty" json:"mic,omitempty"`
}

// pickIndex finds the screen row a pick names: by instrument id, else ISIN and venue, else ticker.
func pickIndex(k PickKey, n int, row func(i int) PickKey) int {
	if k.InstrumentID != "" {
		for i := 0; i < n; i++ {
			if row(i).InstrumentID == k.InstrumentID {
				return i
			}
		}
	}
	if k.ISIN != "" {
		for i := 0; i < n; i++ {
			if r := row(i); strings.EqualFold(r.ISIN, k.ISIN) && strings.EqualFold(r.MIC, k.MIC) && strings.EqualFold(r.Ticker, k.Ticker) {
				return i
			}
		}
	}
	for i := 0; i < n; i++ {
		if strings.EqualFold(row(i).Ticker, k.Ticker) {
			return i
		}
	}
	return -1
}

func keysOf(keys []PickKey, tickers []string) []PickKey {
	if len(keys) > 0 {
		return keys
	}
	out := make([]PickKey, len(tickers))
	for i, t := range tickers {
		out[i] = PickKey{Ticker: t}
	}
	return out
}

func watchlistCmd(e *env) *cobra.Command {
	c := &cobra.Command{Use: "watchlist", Short: "data/watchlist.yaml from the Artifact's picks, and names added later"}
	var run string
	from := &cobra.Command{
		Use:   "from-picks <picks.json|yaml>",
		Short: "Write data/watchlist.yaml from the Screen-page picks and the run's screen.yaml",
		Long: `Reads the picks the Artifact's Screen page saved ({stocks: [tickers], funds: [tickers]}, as the
ArtifactData tool returns the document) and the run's screen.yaml, and writes data/watchlist.yaml with each
name's ISIN, venue, currency, country, sector and the screener's instrument id where the screener gave one.
A ticker not in screen.yaml is an error. Then run your broker connector's sync, if you have one.
Example:
  pb watchlist from-picks data/runs/<date>/picks/<date>.json --run <date>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			var p Picks
			if json.Valid(b) {
				// ArtifactData may hand back the whole document: {data: {...}} or the fields themselves.
				var wrap struct {
					Data *Picks `json:"data"`
				}
				if json.Unmarshal(b, &wrap) == nil && wrap.Data != nil {
					p = *wrap.Data
				} else if err := json.Unmarshal(b, &p); err != nil {
					return err
				}
			} else if err := yaml.Unmarshal(b, &p); err != nil {
				return err
			}
			var doc ScreenDoc
			ok, err := e.readYAML(&doc, "runs", cmp.Or(run, p.Run, e.today()), "screen.yaml")
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no screen.yaml for run %s", cmp.Or(run, p.Run, e.today()))
			}
			w := Watchlist{AsOf: e.today(), Source: "artifact screen picks"}
			var unknown []string
			stockRow := func(i int) PickKey {
				c := doc.Stocks[i]
				return PickKey{Ticker: c.Ticker, InstrumentID: c.InstrumentID, ISIN: c.ISIN, MIC: c.MIC}
			}
			fundRow := func(i int) PickKey {
				c := doc.Funds[i]
				return PickKey{Ticker: c.Ticker, InstrumentID: c.InstrumentID, ISIN: c.ISIN, MIC: cmp.Or(c.MIC, c.Exchange)}
			}
			for _, k := range keysOf(p.StockKeys, p.Stocks) {
				i := pickIndex(k, len(doc.Stocks), stockRow)
				if i < 0 {
					unknown = append(unknown, k.Ticker)
					continue
				}
				c := doc.Stocks[i]
				w.Items = append(w.Items, WatchItem{Ticker: c.Ticker, Name: c.Name, Kind: kindStock, ISIN: c.ISIN, MIC: c.MIC,
					Currency: c.Currency, Country: c.Country, Sector: c.Sector, InstrumentID: c.InstrumentID})
			}
			for _, k := range keysOf(p.FundKeys, p.Funds) {
				i := pickIndex(k, len(doc.Funds), fundRow)
				if i < 0 {
					unknown = append(unknown, k.Ticker)
					continue
				}
				c := doc.Funds[i]
				w.Items = append(w.Items, WatchItem{Ticker: c.Ticker, Name: c.Name, Kind: kindETF, ISIN: c.ISIN, Exchange: c.Exchange,
					MIC: c.MIC, Currency: c.Currency, InstrumentID: c.InstrumentID})
			}
			if len(unknown) > 0 {
				return fmt.Errorf("picks not in screen.yaml: %s", strings.Join(unknown, ", "))
			}
			path, err := e.writeYAML(w, "watchlist.yaml")
			if err != nil {
				return err
			}
			rep := map[string]any{"file": path, "stocks": len(p.Stocks), "funds": len(p.Funds)}
			if n := len(p.Stocks); n < MinPickedStocks || n > MaxPickedStocks {
				rep["warning"] = fmt.Sprintf("%d stocks picked; gate 1 takes %d to %d (never trimmed): ask the investor", n, MinPickedStocks, MaxPickedStocks)
			}
			return e.printJSON(rep)
		},
	}
	from.Flags().StringVar(&run, "run", "", "run date whose screen.yaml to read (default: the picks' run, else today)")
	c.AddCommand(from, watchlistAddCmd(e))
	return c
}

// fundWordExcluded is the first of the strategy's excluded_fund_words a fund's name or asset type
// (lower case) contains, "" when none.
func fundWordExcluded(nameAndType string) string {
	for _, w := range pack.Active().Strategy.ExcludedFundWords {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(nameAndType, w) {
			return w
		}
	}
	return ""
}
