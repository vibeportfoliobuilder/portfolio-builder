package eulerpool

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// ETFProfile is etf/profile; the vendor resolves an ISIN to ONE listing of its choosing.
type ETFProfile struct {
	ISIN          string `json:"isin"`
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	AssetClass    string `json:"assetClass"`
	Issuer        string `json:"etfCompany"`
	ExpenseRatio  Num    `json:"expenseRatio"`
	AUM           Num    `json:"aum"`
	NAV           Num    `json:"nav"`
	Currency      string `json:"currency"`
	LatestQuote   Num    `json:"latestQuotes"`
	Inception     string `json:"inceptionDate"`
	Domicile      string `json:"domicile"`
	DividendYield Num    `json:"dividendYield"`
	UpdatedAt     string `json:"updated_at"`
}

// ETFHolding is one etf/holdings row.
type ETFHolding struct {
	Symbol  string `json:"symbol"`
	Name    string `json:"name"`
	ISIN    string `json:"isin"`
	Percent Num    `json:"percent"`
}

// ETFExposure is one etf/countries or etf/sectors row.
type ETFExposure struct {
	Country  string `json:"country"`
	Sector   string `json:"sector"`
	Exposure Num    `json:"exposure"`
}

// ETFQuote is one [unix ms, price] pair from etf/quotes.
type ETFQuote struct {
	Time  time.Time
	Price float64
}

// ETFDossier is the compact file `pb ep etf` writes; distributions are not in Eulerpool.
type ETFDossier struct {
	AsOf        string      `yaml:"as_of"`
	Source      string      `yaml:"source"`
	ISIN        string      `yaml:"isin"`
	Symbol      string      `yaml:"symbol"`
	Name        string      `yaml:"name"`
	Issuer      string      `yaml:"issuer,omitempty"`
	AssetClass  string      `yaml:"asset_class,omitempty"`
	Currency    string      `yaml:"currency"`
	TER         *float64    `yaml:"ter,omitempty"`
	AUMMillions *float64    `yaml:"aum_millions,omitempty"`
	NAV         *float64    `yaml:"nav,omitempty"`
	Inception   string      `yaml:"inception,omitempty"`
	Domicile    string      `yaml:"domicile,omitempty"`
	DataGaps    []string    `yaml:"data_gaps,omitempty"`
	Price       Price       `yaml:"price"`
	TopHoldings []HolderRow `yaml:"top_holdings,omitempty"`
	// HoldingsOverHalf is every holding of 0.5% or more, with symbol and ISIN, for the look-through.
	HoldingsOverHalf []FundHolding `yaml:"holdings_over_half_pct,omitempty"`
	Countries        []Weight      `yaml:"countries,omitempty"`
	Sectors          []Weight      `yaml:"sectors,omitempty"`
}

// Weight is one exposure in percent.
type Weight struct {
	Name string  `yaml:"name"`
	Pct  float64 `yaml:"pct"`
}

// ETFProfile reads etf/profile.
func (c *Client) ETFProfile(ctx context.Context, id string) (ETFProfile, error) {
	var v ETFProfile
	return v, c.getJSON(ctx, "etf/profile/"+seg(id), nil, TTLFundament, &v)
}

// ETFQuotes reads the full daily quote history.
func (c *Client) ETFQuotes(ctx context.Context, id string) ([]ETFQuote, error) {
	var raw [][2]Num
	if err := c.getJSON(ctx, "etf/quotes/"+seg(id), nil, TTLQuote, &raw); err != nil {
		return nil, err
	}
	out := make([]ETFQuote, 0, len(raw))
	for _, p := range raw {
		if p[0].OK && p[1].OK {
			out = append(out, ETFQuote{Time: time.UnixMilli(int64(p[0].V)).UTC(), Price: p[1].V})
		}
	}
	slices.SortFunc(out, func(a, b ETFQuote) int { return a.Time.Compare(b.Time) })
	return out, nil
}

// ETFHoldings reads all holdings.
func (c *Client) ETFHoldings(ctx context.Context, id string) ([]ETFHolding, error) {
	var v []ETFHolding
	return v, c.getJSON(ctx, "etf/holdings/"+seg(id), nil, TTLFundament, &v)
}

// ETFCountries reads country exposure.
func (c *Client) ETFCountries(ctx context.Context, id string) ([]ETFExposure, error) {
	var v []ETFExposure
	return v, c.getJSON(ctx, "etf/countries/"+seg(id), nil, TTLFundament, &v)
}

// ETFSectors reads sector exposure.
func (c *Client) ETFSectors(ctx context.Context, id string) ([]ETFExposure, error) {
	var v []ETFExposure
	return v, c.getJSON(ctx, "etf/sectors/"+seg(id), nil, TTLFundament, &v)
}

// ETF builds the compact ETF dossier for an ISIN, five requests on a cold cache.
func (c *Client) ETF(ctx context.Context, isin string) (*ETFDossier, error) {
	return c.ETFLine(ctx, isin, Line{})
}

// Line is the fund line the investor trades: the vendor ticker (VALU.L) and its trading currency.
type Line struct{ Symbol, Currency string }

// lineProfile resolves what to quote. The vendor's profile always answers with its own listing for
// the ISIN (asked for VALU.L it returns VALL.L in GBP), but its quotes route serves the line asked for
// (VALU.L's USD prices), so the traded line is quoted when the vendor knows the symbol, in the line's
// currency; the profile (NAV, AUM, TER) stays the vendor's listing, and the note says which is which.
func (c *Client) lineProfile(ctx context.Context, isin string, line Line) (p ETFProfile, quoteID, quoteCcy, note string, err error) {
	if line.Symbol != "" {
		lp, lerr := c.ETFProfile(ctx, line.Symbol)
		if errors.Is(lerr, ErrUnauthorized) {
			return lp, "", "", "", lerr
		}
		if lerr == nil && (lp.ISIN == "" || strings.EqualFold(lp.ISIN, isin)) {
			ccy := cmp.Or(strings.ToUpper(line.Currency), lp.Currency)
			note = fmt.Sprintf("eulerpool: quotes are from %s in %s, the line the broker trades", line.Symbol, ccy)
			if !strings.EqualFold(lp.Symbol, line.Symbol) {
				note += fmt.Sprintf("; NAV, AUM and TER are the vendor's listing %s in %s", lp.Symbol, lp.Currency)
			}
			return lp, line.Symbol, ccy, note, nil
		}
	}
	if p, err = c.ETFProfile(ctx, isin); err != nil {
		return p, "", "", "", err
	}
	note = fmt.Sprintf("eulerpool: the ISIN resolved to listing %s in %s, which may not be the line you trade", p.Symbol, p.Currency)
	if line.Symbol != "" {
		note = fmt.Sprintf("eulerpool: the traded line %s is not at the vendor; the ISIN resolved to listing %s in %s, which may not be the line you trade", line.Symbol, p.Symbol, p.Currency)
	}
	return p, isin, p.Currency, note, nil
}

// ETFLine is ETF on the line the investor trades (symbol, a vendor ticker such as VALU.L), where the
// vendor has it; holdings and
// exposures stay on the ISIN. Two more requests when the line is asked for.
func (c *Client) ETFLine(ctx context.Context, isin string, line Line) (*ETFDossier, error) {
	now := c.now().UTC()
	p, quoteID, quoteCcy, lineNote, err := c.lineProfile(ctx, isin, line)
	if err != nil {
		return nil, err
	}
	sym := p.Symbol
	if quoteID != isin {
		sym = quoteID
	}
	d := &ETFDossier{AsOf: day(now), Source: "eulerpool", ISIN: cmp.Or(p.ISIN, isin), Symbol: sym, Name: p.Name, Issuer: p.Issuer,
		AssetClass: p.AssetClass, Currency: quoteCcy, TER: round(p.ExpenseRatio.Ptr(), 4), NAV: round(p.NAV.Ptr(), 4), Inception: p.Inception, Domicile: p.Domicile}
	if p.AUM.OK {
		d.AUMMillions = round(f(p.AUM.V/1e6), 1)
	}
	s := &fetchState{d: &Dossier{}}
	s.gap("eulerpool: no ETF distributions anywhere in the API (equity/dividends answers 404, dividendYield is null); take the yield from the broker's fund data (broker.yaml)")
	s.gap("%s", lineNote)
	qs, err := c.ETFQuotes(ctx, quoteID)
	if s.soft(ctx, "ETF quotes", err) && len(qs) > 0 {
		scale := 1.0
		last := qs[len(qs)-1].Price
		// The NAV is the profile listing's; the scale check only holds for that listing's quotes.
		if p.NAV.OK && last > 0 && quoteID == isin {
			if r := p.NAV.V / last; r > 50 && r < 200 {
				scale = 100
				s.gap("eulerpool: quotes are 100x below the NAV (GBX/GBP mix-up); they were multiplied by 100")
			} else if r > 0.005 && r < 0.02 {
				scale = 0.01
				s.gap("eulerpool: quotes are 100x above the NAV (GBX); they were divided by 100")
			}
		}
		o := OHLCV{}
		for _, q := range qs {
			o.T = append(o.T, Num{V: float64(q.Time.Unix()), OK: true})
			o.C = append(o.C, Num{V: q.Price, OK: true})
		}
		d.Price.Current = round(f(last*scale), 4)
		d.Price.AsOf = day(qs[len(qs)-1].Time)
		d.Price.fromCloses(monthlyCloses(o, scale))
		for _, g := range quoteGaps(qs, now) {
			s.gap("%s", g)
		}
		if p.NAV.OK && p.NAV.V > 0 && last > 0 && quoteID == isin {
			if r := last * scale / p.NAV.V; r < 1/navTolerance || r > navTolerance {
				s.gap("eulerpool: the price %.4g is %.2f× the NAV %.4g: another line or currency than the NAV's, or a broken series (needs a look)", last*scale, r, p.NAV.V)
			}
		}
	}
	hs, err := c.ETFHoldings(ctx, isin)
	s.soft(ctx, "ETF holdings", err)
	slices.SortStableFunc(hs, func(a, b ETFHolding) int { return cmp.Compare(b.Percent.V, a.Percent.V) })
	for _, h := range hs[:min(15, len(hs))] {
		d.TopHoldings = append(d.TopHoldings, HolderRow{Name: cmp.Or(h.Name, h.Symbol), Pct: round(h.Percent.Ptr(), 2)})
	}
	d.HoldingsOverHalf = holdingsOver(hs, MinLookThroughPct)
	weights := func(rows []ETFExposure, name func(ETFExposure) string) []Weight {
		var out []Weight
		for _, r := range rows {
			if r.Exposure.OK {
				out = append(out, Weight{Name: name(r), Pct: *round(&r.Exposure.V, 2)})
			}
		}
		slices.SortStableFunc(out, func(a, b Weight) int { return cmp.Compare(b.Pct, a.Pct) })
		return out[:min(15, len(out))]
	}
	cs, err := c.ETFCountries(ctx, isin)
	s.soft(ctx, "ETF countries", err)
	d.Countries = weights(cs, func(e ETFExposure) string { return e.Country })
	ss, err := c.ETFSectors(ctx, isin)
	s.soft(ctx, "ETF sectors", err)
	d.Sectors = weights(ss, func(e ETFExposure) string { return e.Sector })
	d.DataGaps = s.d.DataGaps
	if s.fatal != nil {
		return nil, s.fatal
	}
	return d, nil
}

// Quote-series checks.
const (
	quoteGapDays  = 45   // a hole longer than this in daily quotes is a gap
	quoteGapYears = 5    // only the years the returns use
	quoteGapJump  = 1.5  // a price move across a gap beyond this (either way) breaks the returns over it
	navTolerance  = 1.25 // price against NAV
)

// quoteGaps names the holes in a daily quote series over the last years and the price move across each.
func quoteGaps(qs []ETFQuote, now time.Time) []string {
	var out []string
	from := now.AddDate(-quoteGapYears, 0, 0)
	for i := 1; i < len(qs); i++ {
		a, b := qs[i-1], qs[i]
		if b.Time.Before(from) || b.Time.Sub(a.Time) <= quoteGapDays*24*time.Hour || a.Price <= 0 {
			continue
		}
		g := fmt.Sprintf("eulerpool: the quote series has no prices from %s to %s (%d days)", day(a.Time), day(b.Time), int(b.Time.Sub(a.Time).Hours()/24))
		if r := b.Price / a.Price; r > quoteGapJump || r < 1/quoteGapJump {
			g += fmt.Sprintf(" and the price moves %.2f× across it: returns spanning it are unreliable", r)
		}
		out = append(out, g)
	}
	return out
}

// Article is one news item.
type Article struct {
	Date     string `yaml:"date"`
	Headline string `yaml:"headline"`
	Summary  string `yaml:"summary,omitempty"`
	Source   string `yaml:"source,omitempty"`
	URL      string `yaml:"url"`
}

type wireArticle struct {
	Headline string `json:"headline"`
	Summary  string `json:"summary"`
	Source   string `json:"source"`
	URL      string `json:"url"`
	Datetime Num    `json:"datetime"`
}

func articles(ws []wireArticle, n int) []Article {
	slices.SortStableFunc(ws, func(a, b wireArticle) int { return cmp.Compare(b.Datetime.V, a.Datetime.V) })
	var out []Article
	for _, w := range ws {
		if w.Headline == "" || w.URL == "" {
			continue
		}
		out = append(out, Article{Date: day(time.Unix(int64(w.Datetime.V), 0)), Headline: strings.TrimSpace(w.Headline), Summary: clip(w.Summary, 400), Source: w.Source, URL: w.URL})
		if len(out) == n {
			break
		}
	}
	return out
}

// News reads research/news for an ISIN or vendor ticker; non-US feeds stopped in Feb 2026.
func (c *Client) News(ctx context.Context, id string, n int) ([]Article, error) {
	t, err := c.vendorTicker(ctx, id)
	if err != nil {
		return nil, err
	}
	var ws []wireArticle
	if err := c.getJSON(ctx, "research/news/"+seg(t), nil, TTLNews, &ws); err != nil {
		return nil, err
	}
	return articles(ws, n), nil
}

// MarketNews reads the general market feed.
func (c *Client) MarketNews(ctx context.Context, n int) ([]Article, error) {
	var ws []wireArticle
	if err := c.getJSON(ctx, "equity-extended/market-news", url.Values{"limit": {fmt.Sprint(max(n, 1))}}, TTLNews, &ws); err != nil {
		return nil, err
	}
	return articles(ws, n), nil
}

// NewsSentiment is sentiment/news-sentiment (US names only).
type NewsSentiment struct {
	Symbol       string   `yaml:"symbol"`
	Score        *float64 `yaml:"score,omitempty"`
	Bullish      *float64 `yaml:"bullish_share,omitempty"`
	Bearish      *float64 `yaml:"bearish_share,omitempty"`
	ArticlesWeek *float64 `yaml:"articles_last_week,omitempty"`
}

// NewsSentiment reads the vendor's news sentiment; 404 outside the US.
func (c *Client) NewsSentiment(ctx context.Context, id string) (NewsSentiment, error) {
	var w struct {
		Symbol  string `json:"symbol"`
		Score   Num    `json:"company_news_score"`
		Bullish Num    `json:"bullish_percent"`
		Bearish Num    `json:"bearish_percent"`
		Week    Num    `json:"articles_in_last_week"`
	}
	if err := c.getJSON(ctx, "sentiment/news-sentiment/"+seg(id), nil, TTLNews, &w); err != nil {
		return NewsSentiment{}, err
	}
	return NewsSentiment{Symbol: w.Symbol, Score: w.Score.Ptr(), Bullish: w.Bullish.Ptr(), Bearish: w.Bearish.Ptr(), ArticlesWeek: w.Week.Ptr()}, nil
}

// Filing is one SEC filing.
type Filing struct {
	Form  string `yaml:"form"`
	Filed string `yaml:"filed"`
	URL   string `yaml:"url"`
}

// Filings reads equity-extended/sec-filings, newest first; the feed stops in Feb 2026.
func (c *Client) Filings(ctx context.Context, id string) ([]Filing, error) {
	var ws []struct {
		Form      string `json:"form"`
		FiledDate string `json:"filedDate"`
		ReportURL string `json:"reportUrl"`
	}
	if err := c.getJSON(ctx, "equity-extended/sec-filings/"+seg(id), nil, TTLFundament, &ws); err != nil {
		return nil, err
	}
	var out []Filing
	for _, w := range ws {
		if w.Form != "" && w.ReportURL != "" {
			out = append(out, Filing{Form: w.Form, Filed: day10(w.FiledDate), URL: w.ReportURL})
		}
	}
	slices.SortStableFunc(out, func(a, b Filing) int { return strings.Compare(b.Filed, a.Filed) })
	return out, nil
}

// vendorTicker maps an ISIN to the vendor ticker the {ticker} routes need.
func (c *Client) vendorTicker(ctx context.Context, id string) (string, error) {
	if !looksLikeISIN(id) {
		return id, nil
	}
	ov, err := c.Overview(ctx, id)
	if err != nil {
		return "", err
	}
	if ov.Ticker == "" {
		return "", errors.New("eulerpool: no vendor ticker for " + id)
	}
	return ov.Ticker, nil
}

func looksLikeISIN(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i, r := range s {
		switch {
		case i < 2 && (r < 'A' || r > 'Z'):
			return false
		case i >= 2 && !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'):
			return false
		}
	}
	return true
}
