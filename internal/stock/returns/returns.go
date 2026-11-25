// Package returns turns a monthly price series and a dividend ledger into the growth figures
// every name shows: price CAGR, total return CAGR (dividends reinvested, gross
// of withholding) and the market sensitivity the dividend score reads. Pure: no I/O.
package returns

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

// Horizons are the windows every table shows, in years.
var Horizons = []int{1, 5, 10}

// Key is a horizon's map key: "1y", "5y", "10y".
func Key(years int) string { return strconv.Itoa(years) + "y" }

// The one-word reasons a figure is absent. The page prints them after "n/a".
const (
	NoHistory    = "no history"   // the series does not reach back that far
	NoDividend   = "no dividend"  // nothing was paid
	Accumulating = "accumulating" // the fund reinvests inside; no distributions to grow
	NoSource     = "no source"    // no vendor carries the figure
	NotFetched   = "not fetched"  // pb ep returns has not run for the name
)

// Close is one month's last close; Month is "YYYY-MM".
type Close struct {
	Month string  `yaml:"month" json:"month"`
	Close float64 `yaml:"close" json:"close"`
}

// Payment is one dividend by its ex-date ("YYYY-MM-DD"), per share, in the price's currency.
type Payment struct {
	ExDate string  `yaml:"ex_date" json:"ex_date"`
	Amount float64 `yaml:"amount" json:"amount"`
}

// Figures is what one name shows: each map holds "1y", "5y", "10y"; a missing key has its
// reason in NA under "<map>.<key>" (for example "total_return_pct.10y": "no history").
type Figures struct {
	AsOf           string              `yaml:"as_of,omitempty" json:"as_of,omitempty"`
	Source         string              `yaml:"source,omitempty" json:"source,omitempty"`
	Listing        string              `yaml:"listing,omitempty" json:"listing,omitempty"`
	YieldPct       *float64            `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	PriceCAGR      map[string]*float64 `yaml:"price_cagr_pct" json:"price_cagr_pct"`
	TotalReturn    map[string]*float64 `yaml:"total_return_pct" json:"total_return_pct"`
	DividendGrowth map[string]*float64 `yaml:"dividend_growth_pct" json:"dividend_growth_pct"`
	NA             map[string]string   `yaml:"n_a,omitempty" json:"n_a,omitempty"`
	Notes          []string            `yaml:"notes,omitempty" json:"notes,omitempty"`
	// Specials flags relatively large special dividends in the last five years (specials.go).
	Specials *Special `yaml:"special_dividends,omitempty" json:"special_dividends,omitempty"`
	// Streaks are the years with no cut and of consecutive increases (fully paid fiscal years).
	Streaks map[string]int `yaml:"streak_years,omitempty" json:"streak_years,omitempty"`
	// MaxDrawdownPct is the worst fall from a month-end high to a later month-end low over the
	// series (negative, percent), with the two months: the stress figures the agents cite.
	MaxDrawdownPct  *float64 `yaml:"max_drawdown_pct,omitempty" json:"max_drawdown_pct,omitempty"`
	MaxDrawdownFrom string   `yaml:"max_drawdown_from,omitempty" json:"max_drawdown_from,omitempty"`
	MaxDrawdownTo   string   `yaml:"max_drawdown_to,omitempty" json:"max_drawdown_to,omitempty"`
}

// MaxDrawdown is the worst peak-to-trough fall over month-end closes in month order, in percent
// (negative), with the peak and trough months; nil with fewer than two closes.
func MaxDrawdown(cs []Close) (pct *float64, from, to string) {
	if len(cs) < 2 {
		return nil, "", ""
	}
	peak, peakMonth := cs[0].Close, cs[0].Month
	worst := 0.0
	for _, c := range cs[1:] {
		if c.Close > peak {
			peak, peakMonth = c.Close, c.Month
			continue
		}
		if d := (c.Close/peak - 1) * 100; d < worst {
			worst, from, to = d, peakMonth, c.Month
		}
	}
	v := math.Round(worst*100) / 100
	return &v, from, to
}

// TotalReturnNote is what every total-return figure is: the reader must not take it as net.
const TotalReturnNote = "total return reinvests each dividend at its ex-month close, gross of withholding"

// Series is one name's inputs: monthly closes (any order), the current price (zero means the
// last close), the dividend ledger and the per-fiscal-year dividend CAGRs the source computed.
type Series struct {
	Closes         []Close
	Current        float64
	Payments       []Payment
	DividendGrowth map[string]*float64 // "1y", "3y", "5y", "10y" from fully paid fiscal years
	Accumulating   bool                // a fund that pays nothing out
	Distributions  bool                // the source carries distributions (false for every ETF here)
}

// Compute fills Figures from a series. A figure that cannot be computed is absent with its reason.
func Compute(s Series) Figures {
	f := Figures{PriceCAGR: map[string]*float64{}, TotalReturn: map[string]*float64{}, DividendGrowth: map[string]*float64{}, NA: map[string]string{}}
	cs := sorted(s.Closes)
	cur := s.Current
	if cur <= 0 && len(cs) > 0 {
		cur = cs[len(cs)-1].Close
	}
	ps, dropped := plausible(s.Payments, cs)
	if dropped > 0 {
		f.Notes = append(f.Notes, fmt.Sprintf("%d ledger payments over half the ex-month price were left out of the total return (unit error or a one-off)", dropped))
	}
	s.Payments = ps
	paid := len(s.Payments) > 0
	for _, y := range Horizons {
		k := Key(y)
		if v, why := PriceCAGR(cs, cur, y); v != nil {
			f.PriceCAGR[k] = v
		} else {
			f.NA["price_cagr_pct."+k] = why
		}
		switch {
		case s.Accumulating:
			// An accumulating fund's price already carries its reinvested income.
			if v := f.PriceCAGR[k]; v != nil {
				f.TotalReturn[k] = v
			} else {
				f.NA["total_return_pct."+k] = f.NA["price_cagr_pct."+k]
			}
		case !s.Distributions && !paid:
			f.NA["total_return_pct."+k] = NoSource
		default:
			if v, why := TotalReturnCAGR(cs, s.Payments, cur, y); v != nil {
				f.TotalReturn[k] = v
			} else {
				f.NA["total_return_pct."+k] = why
			}
		}
		switch {
		case s.Accumulating:
			f.NA["dividend_growth_pct."+k] = Accumulating
		case s.DividendGrowth[k] != nil:
			f.DividendGrowth[k] = s.DividendGrowth[k]
		case !s.Distributions && !paid:
			f.NA["dividend_growth_pct."+k] = NoSource
		case !paid:
			f.NA["dividend_growth_pct."+k] = NoDividend
		default:
			f.NA["dividend_growth_pct."+k] = NoHistory
		}
	}
	if v := s.DividendGrowth["3y"]; v != nil && !s.Accumulating {
		f.DividendGrowth["3y"] = v
	}
	f.MaxDrawdownPct, f.MaxDrawdownFrom, f.MaxDrawdownTo = MaxDrawdown(cs)
	if len(f.NA) == 0 {
		f.NA = nil
	}
	return f
}

// sorted returns the closes in month order, dropping non-positive ones.
func sorted(cs []Close) []Close {
	out := make([]Close, 0, len(cs))
	for _, c := range cs {
		if c.Close > 0 && len(c.Month) >= 7 {
			out = append(out, Close{Month: c.Month[:7], Close: c.Close})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out
}

// monthIndex is months since year 0, so two months subtract.
func monthIndex(m string) (int, bool) {
	t, err := time.Parse("2006-01", m[:min(7, len(m))])
	if err != nil {
		return 0, false
	}
	return t.Year()*12 + int(t.Month()) - 1, true
}

// startAt finds the close `years` before the last month: that month, else the nearest earlier
// one within a month (a month with no trade). -1 when the series does not reach back.
func startAt(cs []Close, years int) int {
	if len(cs) == 0 {
		return -1
	}
	last, ok := monthIndex(cs[len(cs)-1].Month)
	if !ok {
		return -1
	}
	want := last - 12*years
	for i := len(cs) - 1; i >= 0; i-- {
		m, ok := monthIndex(cs[i].Month)
		if !ok {
			continue
		}
		if m <= want {
			if want-m <= 1 {
				return i
			}
			return -1
		}
	}
	return -1
}

// PriceCAGR is the compound yearly price change over the last `years` to the current price.
func PriceCAGR(cs []Close, current float64, years int) (*float64, string) {
	i := startAt(cs, years)
	if i < 0 || current <= 0 {
		return nil, NoHistory
	}
	return cagr(cs[i].Close, current, years), ""
}

// TotalReturnCAGR buys one share at the close `years` back, reinvests every dividend whose
// ex-date falls in a later month at that month's close, and values the shares at the current
// price. Gross of withholding.
func TotalReturnCAGR(cs []Close, ps []Payment, current float64, years int) (*float64, string) {
	i := startAt(cs, years)
	if i < 0 || current <= 0 {
		return nil, NoHistory
	}
	byMonth := map[string]float64{}
	for _, p := range ps {
		if p.Amount > 0 && len(p.ExDate) >= 7 {
			byMonth[p.ExDate[:7]] += p.Amount
		}
	}
	shares := 1.0
	for j := i + 1; j < len(cs); j++ {
		if d := byMonth[cs[j].Month]; d > 0 {
			price := cs[j].Close
			if j == len(cs)-1 {
				price = current
			}
			shares += shares * d / price
		}
	}
	return cagr(cs[i].Close, shares*current, years), ""
}

func cagr(from, to float64, years int) *float64 {
	if from <= 0 || to <= 0 {
		return nil
	}
	return round(math.Pow(to/from, 1/float64(years))*100-100, 2)
}

// TTMYield is the dividends with an ex-date in the year to the last ex-date over the price; zero
// when nothing was paid for more than 400 days, nil without a price or a ledger.
func TTMYield(ps []Payment, price float64, now time.Time) *float64 {
	if price <= 0 || len(ps) == 0 {
		return nil
	}
	last := ""
	for _, p := range ps {
		if p.ExDate > last {
			last = p.ExDate
		}
	}
	lt, err := time.Parse("2006-01-02", last[:min(10, len(last))])
	if err != nil {
		return nil
	}
	if now.Sub(lt) > 400*24*time.Hour {
		z := 0.0
		return &z
	}
	// 350 days, as the dossier: an annual payer's ex-date drifts a few days earlier each year.
	from := lt.AddDate(0, 0, -350).Format("2006-01-02")
	// The usual count a year, from the three years to the last ex-date: an annual payer whose
	// ex-date moved six weeks earlier must not count two dividends in one trailing year.
	from3 := lt.AddDate(-3, 0, 0).Format("2006-01-02")
	var in []float64
	n3 := 0
	for _, p := range sortedPayments(ps) {
		if p.ExDate > from3 {
			n3++
		}
		if p.ExDate > from {
			in = append(in, p.Amount)
		}
	}
	first, _ := time.Parse("2006-01-02", sortedPayments(ps)[0].ExDate[:10])
	span := math.Min(3, lt.Sub(first).Hours()/24/365.25)
	if freq := int(math.Round(float64(n3) / span)); span >= 2 && freq >= 1 && len(in) > freq {
		in = in[len(in)-freq:]
	}
	var sum float64
	for _, a := range in {
		sum += a
	}
	return round(sum/price*100, 2)
}

// monthlyReturns pairs a name's and a benchmark's simple monthly returns over common months.
func monthlyReturns(a, b []Close) (ra, rb []float64, months []string) {
	am, bm := map[string]float64{}, map[string]float64{}
	for _, c := range sorted(a) {
		am[c.Month] = c.Close
	}
	for _, c := range sorted(b) {
		bm[c.Month] = c.Close
	}
	var common []string
	for m := range am {
		if _, ok := bm[m]; ok {
			common = append(common, m)
		}
	}
	sort.Strings(common)
	for i := 1; i < len(common); i++ {
		p, q := common[i-1], common[i]
		pi, _ := monthIndex(p)
		qi, _ := monthIndex(q)
		if qi-pi != 1 {
			continue
		}
		ra = append(ra, am[q]/am[p]-1)
		rb = append(rb, bm[q]/bm[p]-1)
		months = append(months, q)
	}
	return ra, rb, months
}

// MinBetaMonths is the fewest common monthly returns a beta is struck over: two years.
const MinBetaMonths = 24

// Beta is the name's monthly-return beta against the benchmark over the last `months` common
// months; nil under MinBetaMonths.
func Beta(name, bench []Close, months int) *float64 {
	ra, rb, _ := monthlyReturns(name, bench)
	if len(ra) > months {
		ra, rb = ra[len(ra)-months:], rb[len(rb)-months:]
	}
	if len(ra) < MinBetaMonths {
		return nil
	}
	ma, mb := mean(ra), mean(rb)
	var cov, vb float64
	for i := range ra {
		cov += (ra[i] - ma) * (rb[i] - mb)
		vb += (rb[i] - mb) * (rb[i] - mb)
	}
	if vb == 0 {
		return nil
	}
	return round(cov/vb, 2)
}

// Window is a market correction a name's fall is measured over.
type Window struct {
	Label    string
	From, To string // "YYYY-MM", inclusive
}

// Corrections are the falls the old score read (2008 is beyond an 11-year series and skipped).
var Corrections = []Window{
	{"2008 GFC", "2007-10", "2009-06"},
	{"2020 COVID", "2020-02", "2020-12"},
	{"2022 rate shock", "2021-11", "2023-06"},
}

// ImpliedBeta is the mean, over the corrections both series cover, of the name's
// peak-to-trough fall over the benchmark's; nil where none is covered.
func ImpliedBeta(name, bench []Close, ws []Window) (*float64, []string) {
	var vs []float64
	var used []string
	for _, w := range ws {
		a, ok1 := peakToTrough(sorted(name), w)
		b, ok2 := peakToTrough(sorted(bench), w)
		if !ok1 || !ok2 || b >= 0 {
			continue
		}
		vs = append(vs, a/b)
		used = append(used, fmt.Sprintf("%s: %.0f%% vs %.0f%%", w.Label, a*100, b*100))
	}
	if len(vs) == 0 {
		return nil, nil
	}
	return round(mean(vs), 2), used
}

// peakToTrough is the worst fall from a running peak inside the window, as a fraction (negative);
// false when the series does not cover the whole window.
func peakToTrough(cs []Close, w Window) (float64, bool) {
	if len(cs) == 0 || cs[0].Month > w.From || cs[len(cs)-1].Month < w.To {
		return 0, false
	}
	peak, worst, n := 0.0, 0.0, 0
	for _, c := range cs {
		if c.Month < w.From || c.Month > w.To {
			continue
		}
		n++
		peak = math.Max(peak, c.Close)
		if dd := c.Close/peak - 1; dd < worst {
			worst = dd
		}
	}
	return worst, n >= 2
}

func mean(vs []float64) float64 {
	var s float64
	for _, v := range vs {
		s += v
	}
	return s / float64(len(vs))
}

func round(v float64, dp int) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	p := math.Pow(10, float64(dp))
	r := math.Round(v*p) / p
	return &r
}

// MaxPaymentShare is the largest single payment, as a share of its ex-month close, the total
// return believes: above it the ledger is wrong more often than the company is generous.
const MaxPaymentShare = 0.5

// plausible drops payments over MaxPaymentShare of their ex-month close.
func plausible(ps []Payment, cs []Close) ([]Payment, int) {
	closeOf := map[string]float64{}
	for _, c := range cs {
		closeOf[c.Month] = c.Close
	}
	out := make([]Payment, 0, len(ps))
	n := 0
	for _, p := range ps {
		if c := closeOf[p.ExDate[:min(7, len(p.ExDate))]]; c > 0 && p.Amount/c > MaxPaymentShare {
			n++
			continue
		}
		out = append(out, p)
	}
	return out, n
}

func sortedPayments(ps []Payment) []Payment {
	out := append([]Payment(nil), ps...)
	sort.Slice(out, func(i, j int) bool { return out[i].ExDate < out[j].ExDate })
	return out
}
