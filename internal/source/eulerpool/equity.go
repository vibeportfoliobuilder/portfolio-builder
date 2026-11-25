package eulerpool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Profile is equity/profile: who the company is. Money in millions of Currency.
type Profile struct {
	ISIN        string `json:"isin"`
	Name        string `json:"name"`
	Country     string `json:"country"`
	Currency    string `json:"currency"`
	Sector      string `json:"sector"`
	Industry    string `json:"industry"`
	IPO         string `json:"ipo"`
	Website     string `json:"website"`
	WKN         string `json:"wkn"`
	Description string `json:"description"`
	Employees   Num    `json:"employees"`
	MCap        Num    `json:"mcap"`
	Shares      Num    `json:"shares"`
}

// Overview is equity/overview: the vendor ticker, the delayed price and headline multiples.
type Overview struct {
	ISIN          string `json:"isin"`
	Ticker        string `json:"ticker"`
	Name          string `json:"name"`
	Currency      string `json:"currency"`
	Country       string `json:"country"`
	Price         Num    `json:"price"`
	Week52High    Num    `json:"week52High"`
	Week52Low     Num    `json:"week52Low"`
	PE            Num    `json:"pe"`
	PS            Num    `json:"ps"`
	DividendYield Num    `json:"dividendYield"`
	MarketCap     Num    `json:"marketCap"`
	Shares        Num    `json:"sharesOutstanding"`
	AAQS          Num    `json:"aaqs"`
}

// Dividend is one equity/dividends payment; ExDate is the vendor's "period".
type Dividend struct {
	ExDate  string `json:"period"`
	PayDate string `json:"payDate"`
	Amount  Num    `json:"dividend"`
}

// FYDividend is one equity/dividends-by-fy row.
type FYDividend struct {
	FYStart      string `json:"fiscal_year_start"`
	FYEnd        string `json:"fiscal_year_end"`
	Total        Num    `json:"total_dividends_per_fy"`
	Count        Num    `json:"dividend_count"`
	ExpectedFreq Num    `json:"expected_freq"`
	FullyPaid    string `json:"fully_paid"`
}

// DividendQuality is equity/dividend-quality; its streak counters were wrong in the probe.
type DividendQuality struct {
	Dividend          Num    `json:"dividend"`
	DividendYield     Num    `json:"dividendYield"`
	YearsPaid         Num    `json:"yearsPaid"`
	YearsNotDecreased Num    `json:"yearsNotDecreased"`
	YearsIncreased    Num    `json:"yearsIncreased"`
	PayoutMonths      []int  `json:"payoutMonths"`
	LastExDate        string `json:"lastExDate"`
	LastPayDate       string `json:"lastPayDate"`
}

// DividendSafety is equity/dividend-safety/{ticker}, a dated vendor snapshot.
type DividendSafety struct {
	Ticker                 string `json:"ticker"`
	EarningsPayoutRatio    Num    `json:"earnings_payout_ratio"`
	FCFPayoutRatio         Num    `json:"fcf_payout_ratio"`
	CashFlowCoverage       Num    `json:"cash_flow_coverage"`
	ConsecutiveGrowthYears Num    `json:"consecutive_growth_years"`
	GrowthTrend            Num    `json:"dividend_growth_trend"`
	Score                  Num    `json:"safety_score"`
	Rating                 string `json:"safety_rating"`
	CreatedAt              string `json:"created_at"`
}

// ValuationYear is one fiscal year of equity/valuation-history (German keys: kgv P/E, kuv P/S).
type ValuationYear struct {
	Period string `json:"period"`
	PE     Num    `json:"kgv"`
	PS     Num    `json:"kuv"`
	EV     Num    `json:"kev"`
}

// ValuationHistory keeps the yearly multiples and drops the vendor's daily series.
type ValuationHistory struct {
	Years   []ValuationYear `json:"valuationData"`
	Current struct {
		PE Num `json:"kgv"`
		PS Num `json:"kuv"`
	} `json:"currents"`
}

// Metrics is equity/metrics. ROE/ROA/ROIC are fractions, margins percent, pe/ps 0 when unset.
type Metrics struct {
	Currency  string `json:"currency"`
	Valuation struct {
		PE Num `json:"pe"`
		PS Num `json:"ps"`
		PB Num `json:"pb"`
	} `json:"valuation"`
	Profitability struct {
		ROE             Num `json:"roe"`
		ROA             Num `json:"roa"`
		ROIC            Num `json:"roic"`
		GrossMargin     Num `json:"grossMargin"`
		OperatingMargin Num `json:"operatingMargin"`
		NetMargin       Num `json:"netMargin"`
	} `json:"profitability"`
	PerShare struct {
		EPS           Num `json:"eps"`
		DPS           Num `json:"dps"`
		BPS           Num `json:"bps"`
		DividendYield Num `json:"dividendYield"`
	} `json:"perShare"`
	Leverage struct {
		DebtToEquity Num `json:"debtToEquity"`
	} `json:"leverage"`
}

// Estimate is one equity/estimates row; amounts are in units, not millions; "e" never marks it.
type Estimate struct {
	Period          string `json:"period"`
	Year            Num    `json:"year"`
	Quarter         Num    `json:"quarter"`
	RevenueEstimate Num    `json:"revenueEstimate"`
	RevenueHigh     Num    `json:"revenueHigh"`
	RevenueLow      Num    `json:"revenueLow"`
	RevenueAnalysts Num    `json:"revenueAnalysts"`
	EPSEstimate     Num    `json:"epsEstimate"`
	EPSHigh         Num    `json:"epsHigh"`
	EPSLow          Num    `json:"epsLow"`
	EPSAnalysts     Num    `json:"epsAnalysts"`
	EBITEstimate    Num    `json:"ebitEstimate"`
	EBITAnalysts    Num    `json:"ebitAnalysts"`
}

// PriceTarget is equity/price-target; LastUpdated shows how frozen it is.
type PriceTarget struct {
	Ticker      string `json:"ticker"`
	High        Num    `json:"target_high"`
	Low         Num    `json:"target_low"`
	Mean        Num    `json:"target_mean"`
	Median      Num    `json:"target_median"`
	Analysts    Num    `json:"num_analysts"`
	LastUpdated string `json:"last_updated"`
}

// Holder is one equity/ownership row (institutions and funds, all countries).
type Holder struct {
	Name       string `json:"name"`
	Shares     Num    `json:"share"`
	Change     Num    `json:"change"`
	FilingDate string `json:"filingDate"`
	Percent    Num    `json:"percent"`
}

// FundHolder is one sentiment/fund-ownership row (US funds in practice).
type FundHolder struct {
	FundName     string `json:"fund_name"`
	OwnershipPct Num    `json:"ownership_pct"`
	Shares       Num    `json:"shares"`
	ValueUSD     Num    `json:"value_usd"`
	FilingDate   string `json:"filing_date"`
}

// Form4 is one equity/sec-form4 row (US insiders).
type Form4 struct {
	OwnerName    string `json:"owner_name"`
	IsDirector   bool   `json:"is_director"`
	IsOfficer    bool   `json:"is_officer"`
	OfficerTitle string `json:"officer_title"`
	TxDate       string `json:"tx_date"`
	TxCode       string `json:"tx_code"`
	Shares       Num    `json:"shares"`
	Price        Num    `json:"price_per_share"`
	TotalValue   Num    `json:"total_value"`
	FilingDate   string `json:"filing_date"`
	SECURL       string `json:"sec_url"`
}

// EUInsider is one equity/insider-trades-eu row (MAR notifications).
type EUInsider struct {
	PersonName      string `json:"person_name"`
	Position        string `json:"position"`
	TransactionType string `json:"transaction_type"`
	TransactionDate string `json:"transaction_date"`
	PublicationDate string `json:"publication_date"`
	Volume          Num    `json:"volume"`
	Price           Num    `json:"price"`
	Currency        string `json:"currency"`
}

// OHLCV is charting/ohlcv: parallel arrays, T in unix seconds.
type OHLCV struct {
	T []Num `json:"t"`
	O []Num `json:"o"`
	H []Num `json:"h"`
	L []Num `json:"l"`
	C []Num `json:"c"`
	V []Num `json:"v"`
}

// LTM is equity/financials-ltm: trailing twelve months, money in millions.
type LTM struct {
	AsOf     string `json:"asOf"`
	Income   Row    `json:"income"`
	Balance  Row    `json:"balance"`
	Cashflow Row    `json:"cashflow"`
}

// StatementKind names one statement route.
type StatementKind string

// The six statement routes: annual and quarterly income, balance sheet and cash flow.
const (
	IncomeAnnual     StatementKind = "equity/incomestatement/"
	BalanceAnnual    StatementKind = "equity/balancesheet/"
	CashflowAnnual   StatementKind = "equity/cashflowstatement/"
	IncomeQuarterly  StatementKind = "equity/income-statement-quarterly/"
	BalanceQuarterly StatementKind = "equity/balance-sheet-quarterly/"
	CashQuarterly    StatementKind = "equity/cashflow-statement-quarterly/"
)

// getJSON fetches one route and decodes it into v.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, ttl time.Duration, v any) error {
	b, err := c.get(ctx, path, q, ttl)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("eulerpool: decoding %s: %w", path, err)
	}
	return nil
}

// Profile reads equity/profile for an ISIN or ticker.
func (c *Client) Profile(ctx context.Context, id string) (Profile, error) {
	var v Profile
	return v, c.getJSON(ctx, "equity/profile/"+seg(id), nil, TTLFundament, &v)
}

// Overview reads equity/overview, the source of the vendor ticker.
func (c *Client) Overview(ctx context.Context, id string) (Overview, error) {
	var v Overview
	return v, c.getJSON(ctx, "equity/overview/"+seg(id), nil, TTLQuote, &v)
}

// Statements reads one statement route, sorted by period, estimate rows included.
func (c *Client) Statements(ctx context.Context, id string, kind StatementKind) ([]Row, error) {
	b, err := c.get(ctx, string(kind)+seg(id), nil, TTLFundament)
	if err != nil {
		return nil, err
	}
	rs, err := decodeRows(b)
	if err != nil {
		return nil, fmt.Errorf("eulerpool: decoding %s: %w", kind, err)
	}
	return rs, nil
}

// LTM reads equity/financials-ltm.
func (c *Client) LTM(ctx context.Context, id string) (LTM, error) {
	var v LTM
	return v, c.getJSON(ctx, "equity/financials-ltm/"+seg(id), nil, TTLFundament, &v)
}

// Dividends reads the payment ledger (split-adjusted, no special-dividend flag).
func (c *Client) Dividends(ctx context.Context, id string) ([]Dividend, error) {
	var v []Dividend
	return v, c.getJSON(ctx, "equity/dividends/"+seg(id), nil, TTLFundament, &v)
}

// DividendsByFY reads per-fiscal-year totals.
func (c *Client) DividendsByFY(ctx context.Context, id string) ([]FYDividend, error) {
	var v []FYDividend
	return v, c.getJSON(ctx, "equity/dividends-by-fy/"+seg(id), nil, TTLFundament, &v)
}

// DividendQuality reads equity/dividend-quality.
func (c *Client) DividendQuality(ctx context.Context, id string) (DividendQuality, error) {
	var v DividendQuality
	return v, c.getJSON(ctx, "equity/dividend-quality/"+seg(id), nil, TTLFundament, &v)
}

// DividendSafety reads equity/dividend-safety; it takes the vendor ticker (Overview.Ticker).
func (c *Client) DividendSafety(ctx context.Context, ticker string) (DividendSafety, error) {
	var v DividendSafety
	return v, c.getJSON(ctx, "equity/dividend-safety/"+seg(ticker), nil, TTLFundament, &v)
}

// ValuationHistory reads yearly P/E and P/S.
func (c *Client) ValuationHistory(ctx context.Context, id string) (ValuationHistory, error) {
	var v ValuationHistory
	return v, c.getJSON(ctx, "equity/valuation-history/"+seg(id), nil, TTLFundament, &v)
}

// Metrics reads equity/metrics.
func (c *Client) Metrics(ctx context.Context, id string) (Metrics, error) {
	var v Metrics
	return v, c.getJSON(ctx, "equity/metrics/"+seg(id), nil, TTLFundament, &v)
}

// Estimates reads analyst estimates (annual rows have a null quarter).
func (c *Client) Estimates(ctx context.Context, id string) ([]Estimate, error) {
	var v []Estimate
	return v, c.getJSON(ctx, "equity/estimates/"+seg(id), nil, TTLFundament, &v)
}

// PriceTarget reads the analyst price-target summary.
func (c *Client) PriceTarget(ctx context.Context, id string) (PriceTarget, error) {
	var v PriceTarget
	return v, c.getJSON(ctx, "equity/price-target/"+seg(id), nil, TTLFundament, &v)
}

// Ownership reads the top holders.
func (c *Client) Ownership(ctx context.Context, id string) ([]Holder, error) {
	var v []Holder
	return v, c.getJSON(ctx, "equity/ownership/"+seg(id), nil, TTLFundament, &v)
}

// FundOwnership reads sentiment/fund-ownership (US names only in practice).
func (c *Client) FundOwnership(ctx context.Context, id string) ([]FundHolder, error) {
	var v []FundHolder
	return v, c.getJSON(ctx, "sentiment/fund-ownership/"+seg(id), nil, TTLFundament, &v)
}

// Form4 reads US insider filings.
func (c *Client) Form4(ctx context.Context, id string) ([]Form4, error) {
	var v []Form4
	return v, c.getJSON(ctx, "equity/sec-form4/"+seg(id), nil, TTLFundament, &v)
}

// EUInsiders reads European insider notifications; 404 for names the feed lacks.
func (c *Client) EUInsiders(ctx context.Context, id string) ([]EUInsider, error) {
	var v []EUInsider
	return v, c.getJSON(ctx, "equity/insider-trades-eu/"+seg(id), nil, TTLFundament, &v)
}

// OHLCV reads daily candles from..to; prices are in the listing's quote unit (GBX on LSE).
func (c *Client) OHLCV(ctx context.Context, id string, from, to time.Time) (OHLCV, error) {
	q := url.Values{"resolution": {"D"}, "from": {strconv.FormatInt(from.Unix(), 10)}, "to": {strconv.FormatInt(to.Unix(), 10)}}
	var v OHLCV
	return v, c.getJSON(ctx, "charting/ohlcv/"+seg(id), q, TTLQuote, &v)
}
