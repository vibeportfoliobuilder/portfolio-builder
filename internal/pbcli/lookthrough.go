package pbcli

// lookthrough.go - the investor's names inside each fund: every liked name (strategy
// liked_tickers), watchlist stock and picked stock that a fund holds at more than 0.5%, for the Screen and
// Watchlist pages. Holdings come from pb screen enrich (screen funds), pb ep etf (holdings of 0.5%+,
// else the top 15) and the broker's fund top holdings.

import (
	"sort"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// Favourite is one of the investor's names inside a fund.
type Favourite struct {
	Ticker string  `json:"ticker"`
	Pct    float64 `json:"pct"`
}

type favName struct{ Ticker, Name, ISIN string }

// likedNames are the strategy's liked_tickers (data/profile/strategy.yaml).
func (e *env) likedNames() []string { return pack.Active().Strategy.LikedTickers }

func (e *env) favouriteNames(sc *ScreenDoc, w Watchlist, picked []string) []favName {
	known := map[string]favName{}
	add := func(t, name, isin string) {
		t = strings.ToUpper(t)
		f := known[t]
		f.Ticker = t
		if f.Name == "" {
			f.Name = name
		}
		if f.ISIN == "" {
			f.ISIN = isin
		}
		known[t] = f
	}
	for _, it := range w.Items {
		if it.Kind != kindETF {
			add(it.Ticker, it.Name, it.ISIN)
		}
	}
	if sc != nil {
		for _, c := range sc.Stocks {
			add(c.Ticker, c.Name, c.ISIN)
		}
	}
	var rows struct {
		Rows []struct{ Ticker, Name, ISIN string } `yaml:"rows"`
	}
	_, _ = e.readYAML(&rows, "screen", "stocks.yaml")
	for _, r := range rows.Rows {
		if _, ok := known[strings.ToUpper(r.Ticker)]; ok {
			add(r.Ticker, r.Name, r.ISIN)
		}
	}
	want := map[string]bool{}
	for _, it := range w.Items {
		if it.Kind != kindETF {
			want[strings.ToUpper(it.Ticker)] = true
		}
	}
	for _, t := range append(e.likedNames(), picked...) {
		want[strings.ToUpper(t)] = true
		if _, ok := known[strings.ToUpper(t)]; !ok {
			for _, r := range rows.Rows {
				if strings.EqualFold(r.Ticker, t) {
					add(r.Ticker, r.Name, r.ISIN)
					break
				}
			}
		}
	}
	var out []favName
	for t := range want {
		f := known[t]
		f.Ticker = t
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
	return out
}

var corpWords = map[string]bool{"corp": true, "corporation": true, "inc": true, "plc": true, "co": true, "ltd": true, "limited": true,
	"sa": true, "se": true, "nv": true, "ag": true, "holdings": true, "holding": true, "group": true, "the": true, "company": true,
	"class": true, "a": true, "b": true, "adr": true, "nyrt": true, "public": true, "reit": true}

func normName(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			return r
		}
		return ' '
	}, s)
	var ws []string
	for _, w := range strings.Fields(s) {
		if !corpWords[w] {
			ws = append(ws, w)
		}
	}
	return strings.Join(ws, " ")
}

// sameCompany matches a holding to a name by ISIN, symbol or a name one starts the other with.
func sameCompany(h eulerpool.FundHolding, f favName) bool {
	if h.ISIN != "" && f.ISIN != "" && strings.EqualFold(h.ISIN, f.ISIN) {
		return true
	}
	if h.Symbol != "" && strings.EqualFold(strings.Split(h.Symbol, ".")[0], f.Ticker) {
		return true
	}
	a, b := normName(h.Name), normName(f.Name)
	if len(a) < 4 || len(b) < 4 {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// favouritesIn lists the favourites a fund holds above 0.5%, largest first.
// No holdings at all is nil (no data); holdings with none of the names is an empty list.
func favouritesIn(hs []eulerpool.FundHolding, favs []favName) []Favourite {
	if len(hs) == 0 {
		return nil
	}
	out := []Favourite{}
	seen := map[string]bool{}
	for _, h := range hs {
		if h.Pct <= eulerpool.MinLookThroughPct {
			continue
		}
		for _, f := range favs {
			if !seen[f.Ticker] && sameCompany(h, f) {
				seen[f.Ticker] = true
				out = append(out, Favourite{Ticker: f.Ticker, Pct: h.Pct})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pct > out[j].Pct })
	return out
}

// etfHoldings is a watchlist fund's holdings: Eulerpool's 0.5%+ list, else its top 15, plus
// The broker's top holdings where Eulerpool has none.
func (e *env) etfHoldings(ticker string) []eulerpool.FundHolding {
	var d eulerpool.ETFDossier
	if ok, _ := e.readYAML(&d, "etfs", dirName(ticker), "financial_data.yaml"); ok {
		if len(d.HoldingsOverHalf) > 0 {
			return d.HoldingsOverHalf
		}
		var out []eulerpool.FundHolding
		for _, h := range d.TopHoldings {
			if h.Pct != nil {
				out = append(out, eulerpool.FundHolding{Name: h.Name, Pct: *h.Pct})
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	var ly broker.ETF
	if ok, _ := e.readYAML(&ly, "etfs", dirName(ticker), broker.File); ok && ly.Fund != nil {
		var out []eulerpool.FundHolding
		for _, h := range ly.Fund.TopHoldings {
			out = append(out, eulerpool.FundHolding{Name: h.Name, Pct: h.Pct})
		}
		return out
	}
	return nil
}
