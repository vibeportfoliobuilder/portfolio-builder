package eulerpool

// series.go - the cheap reads behind pb ep returns: an 11-year monthly price series and the
// whole dividend ledger, without the ~20-request dossier. 3 requests per stock, 2 per fund.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// StockSeries is one stock's monthly closes, its full dividend ledger and the per-fiscal-year
// dividend CAGRs (fully paid years, as the dossier computes them).
type StockSeries struct {
	ISIN         string
	Closes       []Close
	LastClose    float64
	PriceAsOf    string
	Payments     []Payment
	DividendCAGR map[string]*float64
	// NoCutYears and IncreaseYears are the streaks from fully paid fiscal years (the Stable groups).
	NoCutYears, IncreaseYears int
	Gaps                      []string
}

// StockSeries reads charting/ohlcv (11 years, the dossier's own window so a same-day fetch is
// a cache hit), equity/dividends and equity/dividends-by-fy. scale converts the quote unit
// (0.01 for an LSE line quoted in pence); the ledger is already in the currency.
func (c *Client) StockSeries(ctx context.Context, isin string, scale float64) (*StockSeries, error) {
	if scale == 0 {
		scale = 1
	}
	now := c.now().UTC()
	today := now.Truncate(24 * time.Hour)
	out := &StockSeries{ISIN: isin, DividendCAGR: map[string]*float64{}}
	o, err := c.OHLCV(ctx, isin, today.AddDate(-priceYears, 0, 0), today.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	out.Closes = monthlyCloses(o, scale)
	if n := alignUnits(out.Closes); n > 0 {
		out.Gaps = append(out.Gaps, fmt.Sprintf("eulerpool: %d monthly closes jumped about 100x against the next month (pence and pounds mixed); they were rescaled to the latest unit", n))
	}
	if n := len(out.Closes); n > 0 {
		out.LastClose = out.Closes[n-1].Close
	}
	if n := len(o.T); n > 0 && o.T[n-1].OK {
		out.PriceAsOf = day(time.Unix(int64(o.T[n-1].V), 0).UTC())
	}
	if len(out.Closes) == 0 {
		return nil, errors.New("eulerpool: no daily prices for " + isin)
	}
	pays, err := c.Dividends(ctx, isin)
	switch {
	case errors.Is(err, ErrNotFound):
		out.Gaps = append(out.Gaps, "eulerpool: no dividend ledger (http 404)")
	case err != nil:
		if errors.Is(err, ErrUnauthorized) || ctx.Err() != nil {
			return nil, err
		}
		out.Gaps = append(out.Gaps, "eulerpool: dividend ledger failed: "+err.Error())
	}
	for _, p := range pays {
		if p.Amount.OK && p.Amount.V > 0 && p.ExDate != "" {
			out.Payments = append(out.Payments, Payment{ExDate: day10(p.ExDate), PayDate: p.PayDate, DPS: *round(&p.Amount.V, 6)})
		}
	}
	slices.SortFunc(out.Payments, func(a, b Payment) int { return strings.Compare(a.ExDate, b.ExDate) })
	if len(out.Payments) == 0 {
		return out, nil
	}
	repaired := 0
	if scale != 1 {
		repaired = repairPence(out.Payments, out.Closes)
		if repaired > 0 {
			out.Gaps = append(out.Gaps, fmt.Sprintf("eulerpool: %d ledger payments were in pence against a GBP price (over a quarter of the price); divided by 100, and dividend growth recomputed from calendar years", repaired))
			out.DividendCAGR = calendarCAGR(out.Payments, now.Year())
			return out, nil
		}
	}
	fy, err := c.DividendsByFY(ctx, isin)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) || ctx.Err() != nil {
			return nil, err
		}
		out.Gaps = append(out.Gaps, "eulerpool: dividends by fiscal year failed: "+err.Error())
		return out, nil
	}
	h := &DividendHistory{}
	for _, r := range fy {
		y, _ := strconv.Atoi(r.FYEnd[:min(4, len(r.FYEnd))])
		if y == 0 || !r.Total.OK {
			continue
		}
		t := FYTotal{FiscalYear: y, Total: r.Total.V, Count: int(r.Count.V), FullyPaid: r.FullyPaid == "YES"}
		if fr := r.ExpectedFreq.V; r.ExpectedFreq.OK && fr > 0 && t.Count > 0 && t.Count != int(fr) && t.FullyPaid {
			t.Normalised = f(t.Total / float64(t.Count) * fr)
		}
		h.ByFY = append(h.ByFY, t)
	}
	slices.SortFunc(h.ByFY, func(a, b FYTotal) int { return a.FiscalYear - b.FiscalYear })
	streaks(h)
	out.DividendCAGR = map[string]*float64{"1y": h.CAGR1y, "3y": h.CAGR3y, "5y": h.CAGR5y, "10y": h.CAGR10y}
	out.NoCutYears, out.IncreaseYears = h.NoCutYears, h.IncreaseStreakYears
	return out, nil
}

// ETFSeries is one fund listing's monthly closes, as the vendor resolves the ISIN.
type ETFSeries struct {
	ISIN      string
	Symbol    string // the listing the vendor picked
	Currency  string
	Domicile  string
	TER       *float64
	Closes    []Close
	LastClose float64
	PriceAsOf string
	Gaps      []string
}

// ETFSeries reads etf/profile and etf/quotes (2 requests). Eulerpool carries no distributions.
func (c *Client) ETFSeries(ctx context.Context, isin string) (*ETFSeries, error) {
	return c.ETFSeriesLine(ctx, isin, Line{})
}

// ETFSeriesLine is ETFSeries on the line the investor trades where the vendor has it (see ETFLine).
func (c *Client) ETFSeriesLine(ctx context.Context, isin string, line Line) (*ETFSeries, error) {
	p, quoteID, quoteCcy, lineNote, err := c.lineProfile(ctx, isin, line)
	if err != nil {
		return nil, err
	}
	out := &ETFSeries{ISIN: isin, Symbol: p.Symbol, Currency: quoteCcy, Domicile: p.Domicile, TER: round(p.ExpenseRatio.Ptr(), 4)}
	if quoteID != isin {
		out.Symbol = quoteID
	}
	if line.Symbol != "" {
		out.Gaps = append(out.Gaps, lineNote)
	}
	qs, err := c.ETFQuotes(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	if len(qs) == 0 {
		return nil, errors.New("eulerpool: no ETF quotes for " + isin)
	}
	o := OHLCV{}
	for _, q := range qs {
		o.T = append(o.T, Num{V: float64(q.Time.Unix()), OK: true})
		o.C = append(o.C, Num{V: q.Price, OK: true})
	}
	out.Gaps = append(out.Gaps, quoteGaps(qs, c.now().UTC())...)
	// A price ratio does not care about GBX against GBP as long as the series keeps one unit.
	out.Closes = monthlyCloses(o, 1)
	if n := alignUnits(out.Closes); n > 0 {
		out.Gaps = append(out.Gaps, fmt.Sprintf("eulerpool: %d monthly closes rescaled by 100 to the latest unit", n))
	}
	if len(out.Closes) > priceYears*12+1 {
		out.Closes = out.Closes[len(out.Closes)-priceYears*12-1:]
	}
	out.LastClose = out.Closes[len(out.Closes)-1].Close
	out.PriceAsOf = day(qs[len(qs)-1].Time)
	return out, nil
}

// repairPence divides by 100 each payment that is over a quarter of its ex-month close but
// plausible in pence: the vendor's LSE ledgers mix pence and pounds (NWG 2018: 2.1538 against
// a 2.35 price). It returns how many it changed.
func repairPence(ps []Payment, cs []Close) int {
	closeOf := map[string]float64{}
	for _, c := range cs {
		closeOf[c.Month] = c.Close
	}
	n := 0
	for i, p := range ps {
		c := closeOf[p.ExDate[:min(7, len(p.ExDate))]]
		if c <= 0 {
			continue
		}
		if p.DPS/c > 0.25 && p.DPS/100/c <= 0.25 {
			ps[i].DPS = math.Round(p.DPS/100*1e6) / 1e6
			n++
		}
	}
	return n
}

// calendarCAGR is the dividend CAGR over 1, 3, 5 and 10 years from calendar-year totals, the
// current (partial) year left out.
func calendarCAGR(ps []Payment, year int) map[string]*float64 {
	tot := map[int]float64{}
	for _, p := range ps {
		y, _ := strconv.Atoi(p.ExDate[:min(4, len(p.ExDate))])
		if y > 0 && y < year {
			tot[y] += p.DPS
		}
	}
	last := year - 1
	out := map[string]*float64{}
	for _, n := range []int{1, 3, 5, 10} {
		a, b := tot[last-n], tot[last]
		if a > 0 && b > 0 {
			out[strconv.Itoa(n)+"y"] = round(f((math.Pow(b/a, 1/float64(n))-1)*100), 2)
		}
	}
	return out
}

// alignUnits walks back from the latest close and rescales by 100 any month that sits 30 to
// 300 times above or below the month after it: the vendor's LSE series switch between pence
// and pounds (LRE 2021). It returns how many closes it changed.
func alignUnits(cs []Close) int {
	n := 0
	for i := len(cs) - 2; i >= 0; i-- {
		next := cs[i+1].Close
		if next <= 0 || cs[i].Close <= 0 {
			continue
		}
		switch r := cs[i].Close / next; {
		case r > 30 && r < 300:
			cs[i].Close /= 100
			n++
		case r < 1.0/30 && r > 1.0/300:
			cs[i].Close *= 100
			n++
		}
	}
	return n
}
