package eulerpool

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Dossier is the compact per-ticker file `pb ep fetch` writes as financial_data.yaml.
type Dossier struct {
	AsOf          string           `yaml:"as_of"`
	Source        string           `yaml:"source"`
	ISIN          string           `yaml:"isin"`
	Ticker        string           `yaml:"ticker"`
	Currency      string           `yaml:"currency"`
	PriceCurrency string           `yaml:"price_currency"`
	Units         string           `yaml:"units"`
	Identity      Identity         `yaml:"identity"`
	DataGaps      []string         `yaml:"data_gaps,omitempty"`
	Price         Price            `yaml:"price"`
	Ratios        Ratios           `yaml:"ratios"`
	Analyst       *Analyst         `yaml:"analyst,omitempty"`
	TTM           *Period          `yaml:"ttm,omitempty"`
	Years         []Period         `yaml:"years,omitempty"`
	Quarters      []Period         `yaml:"quarters,omitempty"`
	Dividends     *DividendHistory `yaml:"dividend_history,omitempty"`
	VendorSafety  *VendorSafety    `yaml:"vendor_dividend_safety,omitempty"`
	Shares        Shares           `yaml:"shares"`
	Insider       *Insider         `yaml:"insider_transactions,omitempty"`
}

// Identity is who the company is.
type Identity struct {
	Name        string   `yaml:"name"`
	Country     string   `yaml:"country"`
	Sector      string   `yaml:"sector,omitempty"`
	Industry    string   `yaml:"industry,omitempty"`
	Website     string   `yaml:"website,omitempty"`
	IPO         string   `yaml:"ipo,omitempty"`
	Employees   *float64 `yaml:"employees,omitempty"`
	AAQS        *float64 `yaml:"aaqs,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

// Price is the delayed quote, its range, and monthly closes reduced from daily candles.
type Price struct {
	Current   *float64 `yaml:"current,omitempty"`
	AsOf      string   `yaml:"price_as_of,omitempty"`
	High52    *float64 `yaml:"52_week_high,omitempty"`
	Low52     *float64 `yaml:"52_week_low,omitempty"`
	MarketCap *float64 `yaml:"market_cap,omitempty"`
	CAGR1y    *float64 `yaml:"cagr_1y,omitempty"`
	CAGR5y    *float64 `yaml:"cagr_5y,omitempty"`
	CAGR10y   *float64 `yaml:"cagr_10y,omitempty"`
	// MonthlyCloses maps "YYYY-MM" to that month's last close; YAML sorts the keys.
	MonthlyCloses map[string]float64 `yaml:"monthly_closes,omitempty"`
}

// Close is the last close of one month.
type Close struct {
	Month string  `yaml:"month"`
	Close float64 `yaml:"close"`
}

// Ratios are the headline figures; pe, ps, pb are market cap over TTM net income, revenue, equity.
type Ratios struct {
	PE             *float64 `yaml:"pe,omitempty"`
	PS             *float64 `yaml:"ps,omitempty"`
	PB             *float64 `yaml:"pb,omitempty"`
	PEGY           *float64 `yaml:"pegy,omitempty"`
	PEGYBasis      string   `yaml:"pegy_basis,omitempty"`
	ROE            *float64 `yaml:"roe,omitempty"`
	ROA            *float64 `yaml:"roa,omitempty"`
	ROIC           *float64 `yaml:"roic,omitempty"`
	GrossMargin    *float64 `yaml:"gross_margin,omitempty"`
	OpMargin       *float64 `yaml:"op_margin,omitempty"`
	NetMargin      *float64 `yaml:"net_margin,omitempty"`
	DebtToEquity   *float64 `yaml:"debt_to_equity,omitempty"`
	DividendYield  *float64 `yaml:"dividend_yield,omitempty"`
	PayoutRatio    *float64 `yaml:"payout_ratio,omitempty"`
	FCFPayoutRatio *float64 `yaml:"fcf_payout_ratio,omitempty"`
	OCFPayoutRatio *float64 `yaml:"ocf_payout_ratio,omitempty"`
	DebtToFCF      *float64 `yaml:"debt_to_fcf,omitempty"`
}

// Analyst is the price-target summary and the next fiscal years' estimates.
type Analyst struct {
	TargetHigh   *float64       `yaml:"target_high,omitempty"`
	TargetLow    *float64       `yaml:"target_low,omitempty"`
	TargetMean   *float64       `yaml:"target_mean,omitempty"`
	TargetMedian *float64       `yaml:"target_median,omitempty"`
	Analysts     *float64       `yaml:"analysts,omitempty"`
	TargetAsOf   string         `yaml:"target_as_of,omitempty"`
	Estimates    []EstimateYear `yaml:"estimates,omitempty"`
}

// EstimateYear is one fiscal year's consensus; revenue and EBIT in millions.
type EstimateYear struct {
	FiscalYear      int      `yaml:"fiscal_year"`
	Revenue         *float64 `yaml:"revenue,omitempty"`
	RevenueAnalysts *float64 `yaml:"revenue_analysts,omitempty"`
	EBIT            *float64 `yaml:"ebit,omitempty"`
	EPS             *float64 `yaml:"eps,omitempty"`
	EPSLow          *float64 `yaml:"eps_low,omitempty"`
	EPSHigh         *float64 `yaml:"eps_high,omitempty"`
	EPSAnalysts     *float64 `yaml:"eps_analysts,omitempty"`
}

// Period is one fiscal year, quarter or the TTM.
type Period struct {
	Period     string           `yaml:"period"`
	FiscalYear int              `yaml:"fiscal_year,omitempty"`
	PeriodEnd  string           `yaml:"period_end"`
	Valuation  *PeriodValuation `yaml:"valuation,omitempty"`
	Statements Statements       `yaml:"statements"`
}

// PeriodValuation is the vendor's P/E and P/S for that year.
type PeriodValuation struct {
	PE *float64 `yaml:"pe,omitempty"`
	PS *float64 `yaml:"ps,omitempty"`
}

// Statements are the lines carried per period, in millions except EPS.
type Statements struct {
	Revenue        *float64 `yaml:"revenue,omitempty"`
	GrossProfit    *float64 `yaml:"gross_profit,omitempty"`
	OpIncome       *float64 `yaml:"op_income,omitempty"`
	NetIncome      *float64 `yaml:"net_income,omitempty"`
	EPSDiluted     *float64 `yaml:"eps_diluted,omitempty"`
	DilutedShares  *float64 `yaml:"diluted_shares,omitempty"`
	DandA          *float64 `yaml:"d_and_a,omitempty"`
	Assets         *float64 `yaml:"assets,omitempty"`
	Liabilities    *float64 `yaml:"liabilities,omitempty"`
	Equity         *float64 `yaml:"equity,omitempty"`
	Cash           *float64 `yaml:"cash_and_st_inv,omitempty"`
	TotalDebt      *float64 `yaml:"total_debt,omitempty"`
	NetDebt        *float64 `yaml:"net_debt,omitempty"`
	OCF            *float64 `yaml:"ocf,omitempty"`
	Capex          *float64 `yaml:"capex,omitempty"`
	FCF            *float64 `yaml:"fcf,omitempty"`
	DividendsPaid  *float64 `yaml:"dividends_paid,omitempty"`
	StockIssuedNet *float64 `yaml:"stock_issued_net,omitempty"`
}

// DividendHistory is the ledger summarised: per-FY totals in full, payments for three years.
type DividendHistory struct {
	Currency            string    `yaml:"currency"`
	PaymentsPerYear     *float64  `yaml:"payments_per_year,omitempty"`
	TTMPerShare         *float64  `yaml:"ttm_per_share,omitempty"`
	Yield               *float64  `yaml:"yield,omitempty"`
	PaymentYears        int       `yaml:"payment_years"`
	NoCutYears          int       `yaml:"no_cut_years"`
	IncreaseStreakYears int       `yaml:"increase_streak_years"`
	CAGR1y              *float64  `yaml:"cagr_1y,omitempty"`
	CAGR3y              *float64  `yaml:"cagr_3y,omitempty"`
	CAGR5y              *float64  `yaml:"cagr_5y,omitempty"`
	CAGR10y             *float64  `yaml:"cagr_10y,omitempty"`
	LedgerSpan          string    `yaml:"ledger_span,omitempty"`
	LastExDate          string    `yaml:"last_ex_date,omitempty"`
	ByFY                []FYTotal `yaml:"by_fy,omitempty"`
	Payments            []Payment `yaml:"payments,omitempty"`
}

// FYTotal is one fiscal year's dividends per share. Normalised rescales a year whose payment
// count is off its frequency (a payment slipping across year end); streaks and CAGRs use it.
type FYTotal struct {
	FiscalYear int      `yaml:"fiscal_year"`
	Total      float64  `yaml:"total"`
	Count      int      `yaml:"count"`
	Normalised *float64 `yaml:"normalised_total,omitempty"`
	FullyPaid  bool     `yaml:"fully_paid"`
}

// value is the total the streaks and CAGRs compare.
func (y FYTotal) value() float64 {
	if y.Normalised != nil {
		return *y.Normalised
	}
	return y.Total
}

// Payment is one ledger entry.
type Payment struct {
	ExDate  string  `yaml:"ex_date"`
	PayDate string  `yaml:"pay_date,omitempty"`
	DPS     float64 `yaml:"dps"`
}

// VendorSafety is the vendor's dated dividend-safety score; only score and rating are trustworthy.
type VendorSafety struct {
	Score  *float64 `yaml:"score,omitempty"`
	Rating string   `yaml:"rating,omitempty"`
	AsOf   string   `yaml:"as_of,omitempty"`
}

// Shares is the count, its trend and the largest holders.
type Shares struct {
	Outstanding     *float64     `yaml:"outstanding,omitempty"`
	ShareCountTrend []ShareCount `yaml:"share_count_trend,omitempty"`
	TopHolders      []HolderRow  `yaml:"top_holders,omitempty"`
	TopFunds        []HolderRow  `yaml:"top_funds,omitempty"`
}

// ShareCount is diluted shares (millions) at a fiscal year end.
type ShareCount struct {
	Period string  `yaml:"period"`
	Shares float64 `yaml:"shares"`
}

// HolderRow is one holder; Pct is percent of shares outstanding.
type HolderRow struct {
	Name   string   `yaml:"name"`
	Pct    *float64 `yaml:"pct,omitempty"`
	Shares *float64 `yaml:"shares,omitempty"`
	Change *float64 `yaml:"change,omitempty"`
	AsOf   string   `yaml:"as_of,omitempty"`
}

// Insider is the insider ledger: a 12-month tally to the latest filing and the recent rows.
type Insider struct {
	Source     string      `yaml:"source"`
	LatestDate string      `yaml:"latest_date,omitempty"`
	Buys12m    int         `yaml:"buys_12m"`
	Sells12m   int         `yaml:"sells_12m"`
	BuyValue   *float64    `yaml:"buy_value_12m,omitempty"`
	SellValue  *float64    `yaml:"sell_value_12m,omitempty"`
	Recent     []InsiderTx `yaml:"recent,omitempty"`
}

// InsiderTx is one transaction.
type InsiderTx struct {
	Date   string   `yaml:"date"`
	Name   string   `yaml:"name"`
	Role   string   `yaml:"role,omitempty"`
	Type   string   `yaml:"type"`
	Shares *float64 `yaml:"shares,omitempty"`
	Price  *float64 `yaml:"price,omitempty"`
	Value  *float64 `yaml:"value,omitempty"`
}

// How much history the dossier keeps where it does not keep everything.
const (
	quarterYears    = 3
	paymentYears    = 3
	priceYears      = 11
	topHolders      = 10
	recentInsiders  = 15
	estimateYears   = 3
	staleAfterDays  = 120
	staleAnnualDays = 460
	ttmWindowDays   = 350
	descriptionMax  = 600
)

// fetchState carries one Fetch: the dossier being built, the vendor-wide gaps that apply and the
// first fatal error.
type fetchState struct {
	d      *Dossier
	vendor []VendorGap
	fatal  error
}

// VendorGap is a gap true of every Eulerpool dossier: its full text is kept once, in
// data/<VendorGapsFile>, and each dossier carries one short line naming the ones that apply.
type VendorGap struct {
	Short string `yaml:"short"`
	Text  string `yaml:"text"`
}

// VendorGapsFile is the file under data/ that holds the vendor-wide gaps' full text.
const VendorGapsFile = "vendor_gaps.yaml"

// The vendor-wide gaps.
var (
	gapAltmanZ = VendorGap{"no Altman Z", "eulerpool: the vendor's Altman Z is always 9999, so none is carried"}
	gapBalance = VendorGap{"annual balance lines from the quarter-end",
		"eulerpool: annual balance-sheet totals are mis-mapped by the vendor (assets = goodwill), so balance lines come from the matching quarter-end; earlier years carry cash and debt only"}
	gapSpecial = VendorGap{"no special-dividend flag",
		"eulerpool: the dividend ledger has no special-dividend flag and is split-adjusted; a one-off payment counts as regular in the totals, streaks and CAGRs"}
)

// VendorGaps are the vendor-wide gaps' full text, for data/<VendorGapsFile>.
func VendorGaps() []VendorGap { return []VendorGap{gapAltmanZ, gapBalance, gapSpecial} }

// vendorGap records a vendor-wide gap; finish turns them into one line.
func (s *fetchState) vendorGap(g VendorGap) { s.vendor = append(s.vendor, g) }

// finishVendorGaps appends the one line that names the vendor-wide gaps this dossier has.
func (s *fetchState) finishVendorGaps() {
	if len(s.vendor) == 0 {
		return
	}
	short := make([]string, len(s.vendor))
	for i, g := range s.vendor {
		short[i] = g.Short
	}
	s.gap("eulerpool (vendor-wide, full text in data/%s): %s", VendorGapsFile, strings.Join(short, "; "))
}

// soft records a failed route as a gap; only a refused key or a cancelled run stops Fetch.
func (s *fetchState) soft(ctx context.Context, what string, err error) bool {
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, ErrUnauthorized) || ctx.Err() != nil:
		if s.fatal == nil {
			s.fatal = err
		}
	case errors.Is(err, ErrNotFound):
		s.gap("eulerpool: no %s for this name (http 404)", what)
	default:
		s.gap("eulerpool: %s failed: %v", what, err)
	}
	return false
}

func (s *fetchState) gap(format string, a ...any) {
	s.d.DataGaps = append(s.d.DataGaps, fmt.Sprintf(format, a...))
}

// Fetch builds the compact dossier for an ISIN or ticker, about 20 requests on a cold cache.
func (c *Client) Fetch(ctx context.Context, id string) (*Dossier, error) {
	now := c.now().UTC()
	prof, err := c.Profile(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("%w: %s is not covered by equity/profile (no preferred shares; ETFs go through ETF)", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	isin := cmp.Or(prof.ISIN, id)
	d := &Dossier{AsOf: day(now), Source: "eulerpool", ISIN: isin, Currency: prof.Currency, PriceCurrency: prof.Currency}
	s := &fetchState{d: d}
	d.Units = fmt.Sprintf("Monetary amounts in millions of %s; prices and per-share figures in %s; share counts in millions; margins, yields, growth, returns and payout in percent; multiples unitless.", prof.Currency, prof.Currency)
	d.Identity = Identity{Name: prof.Name, Country: prof.Country, Sector: prof.Sector, Industry: prof.Industry,
		Website: prof.Website, IPO: prof.IPO, Employees: prof.Employees.Ptr(), Description: clip(prof.Description, descriptionMax)}

	ov, err := c.Overview(ctx, isin)
	s.soft(ctx, "overview (ticker, price)", err)
	d.Ticker = ov.Ticker
	d.Identity.AAQS = ov.AAQS.Ptr()
	scale := 1.0
	if prof.Currency == "GBP" && strings.HasSuffix(ov.Ticker, ".L") {
		scale = 0.01
		s.gap("eulerpool: LSE prices are quoted in GBX (pence); price, 52-week range, market cap, monthly closes and price targets were divided by 100 to GBP")
	}
	s.vendorGap(gapAltmanZ)

	c.fetchStatements(ctx, s, isin, now)
	c.fetchPrice(ctx, s, isin, ov, scale, now)
	c.fetchRatios(ctx, s, isin, ov, scale)
	c.fetchAnalyst(ctx, s, isin, prof.Country, scale, now)
	c.fetchDividends(ctx, s, isin, ov.Ticker, now)
	c.fetchOwnership(ctx, s, isin, prof)
	c.fetchInsider(ctx, s, isin, prof.Country, now)
	finishRatios(d)
	s.finishVendorGaps()
	if s.fatal != nil {
		return nil, s.fatal
	}
	return d, nil
}

// fetchStatements fills Years, Quarters and TTM from the six statement routes and the LTM.
func (c *Client) fetchStatements(ctx context.Context, s *fetchState, isin string, now time.Time) {
	d := s.d
	get := func(k StatementKind, what string) []Row {
		rs, err := c.Statements(ctx, isin, k)
		s.soft(ctx, what, err)
		a, _ := splitActual(rs)
		return a
	}
	is, bs, cf := get(IncomeAnnual, "annual income statement"), get(BalanceAnnual, "annual balance sheet"), get(CashflowAnnual, "annual cash-flow statement")
	isq, bsq, cfq := get(IncomeQuarterly, "quarterly income statement"), get(BalanceQuarterly, "quarterly balance sheet"), get(CashQuarterly, "quarterly cash-flow statement")
	vh, err := c.ValuationHistory(ctx, isin)
	s.soft(ctx, "valuation history", err)
	vals := map[string]ValuationYear{}
	for _, v := range vh.Years {
		vals[v.Period] = v
	}
	byPeriod := func(rs []Row) map[string]Row {
		m := make(map[string]Row, len(rs))
		for _, r := range rs {
			m[r.Str("period")] = r
		}
		return m
	}
	bsA, cfA, bsQ, cfQ := byPeriod(bs), byPeriod(cf), byPeriod(bsq), byPeriod(cfq)
	for _, r := range is {
		p := r.Str("period")
		y, _ := strconv.Atoi(p[:min(4, len(p))])
		per := Period{Period: strconv.Itoa(y), FiscalYear: y, PeriodEnd: p, Statements: lines(r, cfA[p], bsQ[p], bsA[p])}
		if v, ok := vals[p]; ok && (v.PE.OK || v.PS.OK) {
			per.Valuation = &PeriodValuation{PE: round(v.PE.Ptr(), 2), PS: round(v.PS.Ptr(), 2)}
		}
		d.Years = append(d.Years, per)
	}
	if len(d.Years) > 0 {
		s.vendorGap(gapBalance)
		last := d.Years[len(d.Years)-1]
		if t, ok := parseDay(last.PeriodEnd); ok && now.Sub(t) > staleAnnualDays*24*time.Hour {
			s.gap("eulerpool: the latest annual statement is FY%d (period end %s); later fiscal years are missing (known for some small-cap European filersER)", last.FiscalYear, last.PeriodEnd)
		}
		if len(cf) > 0 {
			if lc := cf[len(cf)-1].Str("period"); lc < last.PeriodEnd {
				s.gap("eulerpool: the annual cash-flow statement ends %s while the income statement reaches %s", lc, last.PeriodEnd)
			}
		}
	}
	cut := day(now.AddDate(-quarterYears, 0, 0))
	for _, r := range isq {
		p := r.Str("period")
		if p < cut {
			continue
		}
		q := r.Str("quarterPeriod")
		if q == "" {
			q = quarterName(p)
		}
		d.Quarters = append(d.Quarters, Period{Period: q, PeriodEnd: p, Statements: lines(r, cfQ[p], bsQ[p], nil)})
	}
	ltm, err := c.LTM(ctx, isin)
	if s.soft(ctx, "trailing-twelve-month financials", err) {
		var lastQ Row
		if len(bsq) > 0 {
			lastQ = bsq[len(bsq)-1]
		}
		end := ltm.AsOf
		if len(isq) > 0 {
			end = isq[len(isq)-1].Str("period")
		}
		st := Statements{
			Revenue:       round(ltm.Income.first("revenue"), 2),
			GrossProfit:   round(ltm.Income.first("grossIncome", "grossProfit"), 2),
			OpIncome:      round(ltm.Income.first("ebit", "operatingIncome"), 2),
			NetIncome:     round(ltm.Income.first("netIncome", "bottomLineNetIncome"), 2),
			EPSDiluted:    round(ltm.Income.first("epsDiluted"), 4),
			OCF:           round(ltm.Cashflow.first("netOperatingCashFlow", "operatingCashFlow"), 2),
			Capex:         round(ltm.Cashflow.first("capex", "capitalExpenditure"), 2),
			FCF:           round(ltm.Cashflow.first("fcf", "freeCashFlow"), 2),
			DividendsPaid: round(neg(ltm.Cashflow.first("cashDividendsPaid", "netDividendsPaid", "commonDividendsPaid")), 2),
		}
		balance(&st, lastQ, nil)
		d.TTM = &Period{Period: "TTM", PeriodEnd: end, Statements: st}
	}
}

// lines maps one period's income, cash-flow and balance rows onto Statements.
func lines(is, cf, bsQ, bsA Row) Statements {
	st := Statements{
		Revenue:       round(is.Num("revenue"), 2),
		GrossProfit:   round(is.Num("grossIncome"), 2),
		OpIncome:      round(is.Num("ebit"), 2),
		NetIncome:     round(is.Num("netIncome"), 2),
		EPSDiluted:    round(is.Num("diluted_eps"), 4),
		DilutedShares: round(is.first("dilutedAverageSharesOutstanding", "shares"), 2),
		DandA:         round(is.Num("depreciationAmortization"), 2),
	}
	if cf != nil {
		st.OCF = round(cf.Num("netOperatingCashFlow"), 2)
		st.Capex = round(cf.Num("capex"), 2)
		st.FCF = round(cf.Num("fcf"), 2)
		st.DividendsPaid = round(neg(cf.Num("cashDividendsPaid")), 2)
		st.StockIssuedNet = round(cf.Num("issuanceReductionCapitalStock"), 2)
	}
	balance(&st, bsQ, bsA)
	return st
}

// balance fills the balance lines from a quarter-end row, else cash and debt from the annual row.
func balance(st *Statements, q, a Row) {
	if q != nil {
		st.Assets = round(q.Num("totalAssets"), 2)
		st.Liabilities = round(q.Num("totalLiabilities"), 2)
		st.Equity = round(q.Num("totalEquity"), 2)
		st.Cash = round(q.first("cashShortTermInvestments", "cash"), 2)
		st.TotalDebt = round(sum(q.Num("shortTermDebt"), q.Num("longTermDebt")), 2)
	} else if a != nil {
		st.Cash = round(a.Num("cashShortTermInvestments"), 2)
		st.TotalDebt = round(sum(a.Num("shortTermDebt"), a.Num("currentPortionLongTermDebt"), a.Num("longTermDebt")), 2)
	}
	if st.TotalDebt != nil {
		st.NetDebt = round(sum(st.TotalDebt, neg(st.Cash)), 2)
	}
}

// fetchPrice reduces 11 years of daily candles to monthly closes and price CAGRs.
func (c *Client) fetchPrice(ctx context.Context, s *fetchState, isin string, ov Overview, scale float64, now time.Time) {
	p := &s.d.Price
	sc := func(n Num, dp int) *float64 {
		if !n.OK {
			return nil
		}
		return round(f(n.V*scale), dp)
	}
	p.Current, p.High52, p.Low52, p.MarketCap = sc(ov.Price, 4), sc(ov.Week52High, 4), sc(ov.Week52Low, 4), sc(ov.MarketCap, 0)
	today := now.Truncate(24 * time.Hour)
	o, err := c.OHLCV(ctx, isin, today.AddDate(-priceYears, 0, 0), today.Add(24*time.Hour))
	if !s.soft(ctx, "daily prices", err) {
		return
	}
	if n := len(o.T); n > 0 && o.T[n-1].OK {
		p.AsOf = day(time.Unix(int64(o.T[n-1].V), 0).UTC())
	}
	p.fromCloses(monthlyCloses(o, scale))
}

// fromCloses sets the monthly map and the 1/5/10-year price CAGRs to the current price.
func (p *Price) fromCloses(months []Close) {
	if len(months) == 0 {
		return
	}
	if len(months) > priceYears*12+1 {
		months = months[len(months)-priceYears*12-1:]
	}
	p.MonthlyCloses = make(map[string]float64, len(months))
	for _, m := range months {
		p.MonthlyCloses[m.Month] = m.Close
	}
	if p.Current == nil {
		p.Current = f(months[len(months)-1].Close)
	}
	cagr := func(years int) *float64 {
		i := len(months) - 1 - 12*years
		if i < 0 || months[i].Close <= 0 {
			return nil
		}
		return round(f((math.Pow(*p.Current/months[i].Close, 1/float64(years))-1)*100), 2)
	}
	p.CAGR1y, p.CAGR5y, p.CAGR10y = cagr(1), cagr(5), cagr(10)
}

// monthlyCloses keeps the last close of each calendar month.
func monthlyCloses(o OHLCV, scale float64) []Close {
	var out []Close
	for i, t := range o.T {
		if !t.OK || i >= len(o.C) || !o.C[i].OK {
			continue
		}
		m := time.Unix(int64(t.V), 0).UTC().Format("2006-01")
		v := *round(f(o.C[i].V*scale), 4)
		if n := len(out); n > 0 && out[n-1].Month == m {
			out[n-1].Close = v
		} else {
			out = append(out, Close{Month: m, Close: v})
		}
	}
	return out
}

// fetchRatios fills the vendor's multiples and returns; payout and Debt/FCF come in finishRatios.
func (c *Client) fetchRatios(ctx context.Context, s *fetchState, isin string, ov Overview, scale float64) {
	r := &s.d.Ratios
	m, err := c.Metrics(ctx, isin)
	s.soft(ctx, "metrics", err)
	// The vendor's own multiples are 100x off on GBX listings, so they are only the fallback.
	if scale == 1 {
		r.PE = round(cmp.Or(nonzero(ov.PE.Ptr()), nonzero(m.Valuation.PE.Ptr())), 2)
		r.PS = round(cmp.Or(nonzero(ov.PS.Ptr()), nonzero(m.Valuation.PS.Ptr())), 2)
		r.PB = round(nonzero(m.Valuation.PB.Ptr()), 2)
	}
	hundred := func(n Num) *float64 {
		if !n.OK {
			return nil
		}
		return round(f(n.V*100), 2)
	}
	r.ROE, r.ROA, r.ROIC = hundred(m.Profitability.ROE), hundred(m.Profitability.ROA), hundred(m.Profitability.ROIC)
	r.GrossMargin, r.OpMargin, r.NetMargin = round(m.Profitability.GrossMargin.Ptr(), 2), round(m.Profitability.OperatingMargin.Ptr(), 2), round(m.Profitability.NetMargin.Ptr(), 2)
	r.DebtToEquity = round(m.Leverage.DebtToEquity.Ptr(), 2)
}

// finishRatios derives payout, Debt/FCF, yield and PEGY from the TTM lines and estimates.
func finishRatios(d *Dossier) {
	r := &d.Ratios
	if t := d.TTM; t != nil {
		st := t.Statements
		if mc := d.Price.MarketCap; mc != nil {
			own := func(den *float64) *float64 {
				if den == nil || *den <= 0 {
					return nil
				}
				return round(ratio(mc, den), 2)
			}
			r.PE, r.PS, r.PB = cmp.Or(own(st.NetIncome), r.PE), cmp.Or(own(st.Revenue), r.PS), cmp.Or(own(st.Equity), r.PB)
		}
		if st.DividendsPaid != nil && *st.DividendsPaid > 0 {
			r.PayoutRatio = round(pct(st.DividendsPaid, st.NetIncome), 2)
			r.FCFPayoutRatio = round(pct(st.DividendsPaid, st.FCF), 2)
			r.OCFPayoutRatio = round(pct(st.DividendsPaid, st.OCF), 2)
		}
		if st.FCF != nil && *st.FCF > 0 {
			r.DebtToFCF = round(ratio(st.TotalDebt, st.FCF), 2)
		}
	}
	if d.Dividends != nil {
		r.DividendYield = d.Dividends.Yield
	}
	if r.PE == nil || d.Analyst == nil || len(d.Years) == 0 {
		return
	}
	last := d.Years[len(d.Years)-1]
	if last.Statements.EPSDiluted == nil || *last.Statements.EPSDiluted <= 0 {
		return
	}
	var est *EstimateYear
	for i := range d.Analyst.Estimates {
		if e := &d.Analyst.Estimates[i]; e.FiscalYear <= last.FiscalYear+2 && e.EPS != nil {
			est = e
		}
	}
	if est == nil || *est.EPS <= 0 {
		return
	}
	n := float64(est.FiscalYear - last.FiscalYear)
	g := (math.Pow(*est.EPS / *last.Statements.EPSDiluted, 1/n) - 1) * 100
	// An unknown yield leaves PEGY unknown: a 0 would read as a non-payer.
	if r.DividendYield == nil {
		return
	}
	y := *r.DividendYield
	if g+y <= 0 {
		return
	}
	r.PEGY = round(f(*r.PE/(g+y)), 2)
	r.PEGYBasis = fmt.Sprintf("pe / (eps cagr FY%d actual to FY%d estimate %.1f%% + yield %.1f%%)", last.FiscalYear, est.FiscalYear, g, y)
}

// fetchAnalyst reads the price target and the next fiscal years' annual estimates.
func (c *Client) fetchAnalyst(ctx context.Context, s *fetchState, isin, country string, scale float64, now time.Time) {
	a := &Analyst{}
	pt, err := c.PriceTarget(ctx, isin)
	if s.soft(ctx, "price target", err) {
		ts := 1.0
		if scale != 1 && pt.Mean.OK && s.d.Price.Current != nil && pt.Mean.V > 20**s.d.Price.Current {
			ts = scale
		}
		sc := func(n Num) *float64 {
			if !n.OK {
				return nil
			}
			return round(f(n.V*ts), 4)
		}
		a.TargetHigh, a.TargetLow, a.TargetMean, a.TargetMedian = sc(pt.High), sc(pt.Low), sc(pt.Mean), sc(pt.Median)
		a.Analysts = pt.Analysts.Ptr()
		a.TargetAsOf = day10(pt.LastUpdated)
		if t, ok := parseDay(a.TargetAsOf); ok && now.Sub(t) > staleAfterDays*24*time.Hour {
			s.gap("eulerpool: analyst price target last updated %s", a.TargetAsOf)
		}
	}
	if country != "US" {
		s.gap("eulerpool: non-US analyst estimates and price targets froze in Feb–Mar 2026; treat them as that date's view")
	}
	ests, err := c.Estimates(ctx, isin)
	s.soft(ctx, "analyst estimates", err)
	lastFY := 0
	if n := len(s.d.Years); n > 0 {
		lastFY = s.d.Years[n-1].FiscalYear
	}
	mil := func(n Num) *float64 {
		if !n.OK {
			return nil
		}
		return round(f(n.V/1e6), 2)
	}
	for _, e := range ests {
		y := int(e.Year.V)
		if e.Quarter.OK || !e.Year.OK || y <= lastFY {
			continue
		}
		a.Estimates = append(a.Estimates, EstimateYear{FiscalYear: y, Revenue: mil(e.RevenueEstimate), RevenueAnalysts: e.RevenueAnalysts.Ptr(),
			EBIT: mil(e.EBITEstimate), EPS: round(e.EPSEstimate.Ptr(), 4), EPSLow: round(e.EPSLow.Ptr(), 4), EPSHigh: round(e.EPSHigh.Ptr(), 4), EPSAnalysts: e.EPSAnalysts.Ptr()})
	}
	slices.SortFunc(a.Estimates, func(x, y EstimateYear) int { return x.FiscalYear - y.FiscalYear })
	a.Estimates = a.Estimates[:min(estimateYears, len(a.Estimates))]
	if a.TargetMean != nil || len(a.Estimates) > 0 {
		s.d.Analyst = a
	}
}

// fetchDividends summarises the ledger and the per-FY totals; specials are not flagged by the vendor.
func (c *Client) fetchDividends(ctx context.Context, s *fetchState, isin, ticker string, now time.Time) {
	pays, err := c.Dividends(ctx, isin)
	s.soft(ctx, "dividend ledger", err)
	var ledger []Dividend
	for _, p := range pays {
		if p.Amount.OK && p.Amount.V > 0 && p.ExDate != "" {
			ledger = append(ledger, p)
		}
	}
	if len(ledger) == 0 {
		return
	}
	slices.SortFunc(ledger, func(a, b Dividend) int { return strings.Compare(a.ExDate, b.ExDate) })
	h := &DividendHistory{Currency: s.d.Currency, LedgerSpan: year4(ledger[0].ExDate) + ".." + year4(ledger[len(ledger)-1].ExDate), LastExDate: ledger[len(ledger)-1].ExDate}
	// The ledger lags payments by weeks, so the trailing year ends at the last ex-date, not today.
	lastEx, _ := parseDay(h.LastExDate)
	from, cut := day(lastEx.AddDate(0, 0, -ttmWindowDays)), day(now.AddDate(-paymentYears, 0, 0))
	var ttm float64
	for _, p := range ledger {
		if p.ExDate > from {
			ttm += p.Amount.V
		}
		if p.ExDate > cut {
			h.Payments = append(h.Payments, Payment{ExDate: p.ExDate, PayDate: p.PayDate, DPS: *round(&p.Amount.V, 6)})
		}
	}
	if now.Sub(lastEx) > 400*24*time.Hour {
		ttm = 0
		s.gap("eulerpool: no dividend since %s; the trailing yield is zero", h.LastExDate)
	}
	h.TTMPerShare = round(&ttm, 6)
	if p := s.d.Price.Current; p != nil && *p > 0 {
		h.Yield = round(f(ttm / *p * 100), 2)
	}
	fy, err := c.DividendsByFY(ctx, isin)
	s.soft(ctx, "dividends by fiscal year", err)
	for _, r := range fy {
		y, _ := strconv.Atoi(r.FYEnd[:min(4, len(r.FYEnd))])
		if y == 0 || !r.Total.OK {
			continue
		}
		t := FYTotal{FiscalYear: y, Total: *round(&r.Total.V, 6), Count: int(r.Count.V), FullyPaid: r.FullyPaid == "YES"}
		if fr := r.ExpectedFreq.V; r.ExpectedFreq.OK && fr > 0 && t.Count > 0 && t.Count != int(fr) && t.FullyPaid {
			t.Normalised = round(f(t.Total/float64(t.Count)*fr), 6)
		}
		h.ByFY = append(h.ByFY, t)
		if h.PaymentsPerYear == nil && r.ExpectedFreq.OK {
			h.PaymentsPerYear = r.ExpectedFreq.Ptr()
		}
	}
	slices.SortFunc(h.ByFY, func(a, b FYTotal) int { return a.FiscalYear - b.FiscalYear })
	streaks(h)
	s.d.Dividends = h
	s.vendorGap(gapSpecial)
	if ticker == "" {
		return
	}
	vs, err := c.DividendSafety(ctx, ticker)
	if s.soft(ctx, "vendor dividend safety", err) && (vs.Score.OK || vs.Rating != "") {
		s.d.VendorSafety = &VendorSafety{Score: vs.Score.Ptr(), Rating: vs.Rating, AsOf: day10(vs.CreatedAt)}
		s.gap("eulerpool: the vendor dividend-safety snapshot is dated %s, its payout fields are empty and its growth-year count unreliable; only score and rating are carried", day10(vs.CreatedAt))
	}
}

// streaks fills the payment, no-cut and increase streaks and the CAGRs from fully paid years.
func streaks(h *DividendHistory) {
	var full []FYTotal
	for _, y := range h.ByFY {
		if y.FullyPaid {
			full = append(full, y)
		}
	}
	n := len(full)
	if n == 0 {
		return
	}
	for i := n - 1; i >= 0 && full[i].value() > 0; i-- {
		if i < n-1 && full[i].FiscalYear != full[i+1].FiscalYear-1 {
			break
		}
		h.PaymentYears++
	}
	for i := n - 1; i > 0 && full[i].FiscalYear == full[i-1].FiscalYear+1 && full[i-1].value() > 0; i-- {
		if full[i].value() < full[i-1].value()*0.995 {
			break
		}
		h.NoCutYears++
	}
	for i := n - 1; i > 0 && full[i].FiscalYear == full[i-1].FiscalYear+1 && full[i-1].value() > 0; i-- {
		if full[i].value() <= full[i-1].value()*1.001 {
			break
		}
		h.IncreaseStreakYears++
	}
	last := full[n-1]
	cagr := func(years int) *float64 {
		for _, y := range full {
			if y.FiscalYear == last.FiscalYear-years && y.value() > 0 && last.value() > 0 {
				return round(f((math.Pow(last.value()/y.value(), 1/float64(years))-1)*100), 2)
			}
		}
		return nil
	}
	h.CAGR1y, h.CAGR3y, h.CAGR5y, h.CAGR10y = cagr(1), cagr(3), cagr(5), cagr(10)
}

// fetchOwnership fills the share count, its trend and the top holders.
func (c *Client) fetchOwnership(ctx context.Context, s *fetchState, isin string, prof Profile) {
	sh := &s.d.Shares
	sh.Outstanding = round(prof.Shares.Ptr(), 2)
	start := max(0, len(s.d.Years)-10)
	for _, y := range s.d.Years[start:] {
		if y.Statements.DilutedShares != nil {
			sh.ShareCountTrend = append(sh.ShareCountTrend, ShareCount{Period: y.PeriodEnd, Shares: *y.Statements.DilutedShares})
		}
	}
	hs, err := c.Ownership(ctx, isin)
	s.soft(ctx, "ownership", err)
	slices.SortStableFunc(hs, func(a, b Holder) int { return cmp.Compare(b.Percent.V, a.Percent.V) })
	for _, h := range hs[:min(topHolders, len(hs))] {
		sh.TopHolders = append(sh.TopHolders, HolderRow{Name: h.Name, Pct: round(h.Percent.Ptr(), 2), Shares: round(h.Shares.Ptr(), 0), Change: round(h.Change.Ptr(), 0), AsOf: h.FilingDate})
	}
	if prof.Country != "US" {
		return
	}
	fs, err := c.FundOwnership(ctx, isin)
	s.soft(ctx, "fund ownership", err)
	slices.SortStableFunc(fs, func(a, b FundHolder) int { return cmp.Compare(b.OwnershipPct.V, a.OwnershipPct.V) })
	for _, h := range fs[:min(topHolders, len(fs))] {
		sh.TopFunds = append(sh.TopFunds, HolderRow{Name: h.FundName, Pct: round(h.OwnershipPct.Ptr(), 2), Shares: round(h.Shares.Ptr(), 0), AsOf: h.FilingDate})
	}
}

// fetchInsider reads Form 4 for US names and the EU feed otherwise.
func (c *Client) fetchInsider(ctx context.Context, s *fetchState, isin, country string, now time.Time) {
	var txs []InsiderTx
	src := "SEC Form 4"
	if country == "US" {
		rows, err := c.Form4(ctx, isin)
		s.soft(ctx, "SEC Form 4 insider filings", err)
		for _, r := range rows {
			role := r.OfficerTitle
			if role == "" && r.IsDirector {
				role = "Director"
			}
			txs = append(txs, InsiderTx{Date: r.TxDate, Name: r.OwnerName, Role: role, Type: form4Type(r.TxCode), Shares: r.Shares.Ptr(), Price: round(r.Price.Ptr(), 4), Value: round(r.TotalValue.Ptr(), 0)})
		}
	} else {
		src = "EU insider notifications"
		rows, err := c.EUInsiders(ctx, isin)
		if err != nil && !errors.Is(err, ErrNotFound) {
			s.soft(ctx, "EU insider notifications", err)
		}
		for _, r := range rows {
			var val *float64
			if r.Volume.OK && r.Price.OK && r.Price.V > 0 {
				val = round(f(r.Volume.V*r.Price.V), 0)
			}
			txs = append(txs, InsiderTx{Date: r.TransactionDate, Name: r.PersonName, Role: r.Position, Type: euType(r.TransactionType), Shares: r.Volume.Ptr(), Price: round(r.Price.Ptr(), 4), Value: val})
		}
	}
	if len(txs) == 0 {
		s.gap("eulerpool: no insider transactions for this name (Form 4 is US-only and the EU feed does not cover every issuer)")
		return
	}
	slices.SortStableFunc(txs, func(a, b InsiderTx) int { return strings.Compare(b.Date, a.Date) })
	in := &Insider{Source: src, LatestDate: txs[0].Date}
	from := day10(txs[0].Date)
	if t, ok := parseDay(from); ok {
		from = day(t.AddDate(-1, 0, 0))
		if now.Sub(t) > staleAfterDays*24*time.Hour {
			s.gap("eulerpool: the insider feed ends %s; later transactions are missing", day10(txs[0].Date))
		}
	}
	var bv, sv float64
	for _, t := range txs {
		if t.Date <= from {
			continue
		}
		switch t.Type {
		case "buy":
			in.Buys12m++
			bv += absOr(t.Value)
		case "sell":
			in.Sells12m++
			sv += absOr(t.Value)
		}
	}
	in.BuyValue, in.SellValue = round(&bv, 0), round(&sv, 0)
	in.Recent = txs[:min(recentInsiders, len(txs))]
	s.d.Insider = in
}

func form4Type(code string) string {
	switch code {
	case "P":
		return "buy"
	case "S":
		return "sell"
	case "M", "X":
		return "option exercise"
	case "A":
		return "award"
	case "F":
		return "tax withholding"
	case "G":
		return "gift"
	}
	return "other " + code
}

func euType(t string) string {
	switch strings.ToLower(t) {
	case "acquisition", "purchase", "buy":
		return "buy"
	case "disposal", "sale", "sell":
		return "sell"
	}
	return strings.ToLower(cmp.Or(t, "other"))
}

func absOr(v *float64) float64 {
	if v == nil {
		return 0
	}
	return math.Abs(*v)
}

func year4(s string) string { return s[:min(4, len(s))] }

func day(t time.Time) string { return t.UTC().Format("2006-01-02") }

// day10 is the date part of a vendor timestamp.
func day10(s string) string { return s[:min(10, len(s))] }

func parseDay(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", day10(s))
	return t, err == nil
}

// quarterName labels a quarter-end date like the vendor does ("2026 Q2").
func quarterName(p string) string {
	t, ok := parseDay(p)
	if !ok {
		return p
	}
	return fmt.Sprintf("%d Q%d", t.Year(), (int(t.Month())+2)/3)
}

// clip shortens a description to its first paragraph, cut at a sentence end before n bytes.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i > 0 {
		s = s[:i]
	}
	if len(s) <= n {
		return s
	}
	if i := strings.LastIndex(s[:n], ". "); i > 0 {
		return s[:i+1]
	}
	return s[:n] + "..."
}
