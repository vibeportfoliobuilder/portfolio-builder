// Package broker is the normalized shape of what a broker connector writes per instrument:
// data/stocks/<T>/broker.yaml and data/etfs/<T>/broker.yaml. A connector (the Lightyear example
// screener is public; account connectors are built at setup) decodes its own API into these
// types; pb only reads them. Every field is optional: a connector fills what its broker serves.
package broker

// File is the per-instrument file a connector writes.
const File = "broker.yaml"

// Stock is data/stocks/<T>/broker.yaml.
type Stock struct {
	AsOf         string           `yaml:"as_of" json:"as_of"`
	Source       string           `yaml:"source" json:"source"`
	Instrument   Instrument       `yaml:"instrument" json:"instrument"`
	Company      *Company         `yaml:"company,omitempty" json:"company,omitempty"`
	EarningsCall *EarningsCall    `yaml:"earnings_call,omitempty" json:"earnings_call,omitempty"`
	Events       []CorporateEvent `yaml:"upcoming_events,omitempty" json:"upcoming_events,omitempty"`
	News         []NewsItem       `yaml:"news,omitempty" json:"news,omitempty"`
}

// ETF is data/etfs/<T>/broker.yaml.
type ETF struct {
	AsOf       string           `yaml:"as_of" json:"as_of"`
	Source     string           `yaml:"source" json:"source"`
	Instrument Instrument       `yaml:"instrument" json:"instrument"`
	Fund       *Fund            `yaml:"fund,omitempty" json:"fund,omitempty"`
	Events     []CorporateEvent `yaml:"upcoming_events,omitempty" json:"upcoming_events,omitempty"`
	News       []NewsItem       `yaml:"news,omitempty" json:"news,omitempty"`
}

// Instrument is the broker's listing: its id, symbol, venue and trading currency.
type Instrument struct {
	ID         string `yaml:"id" json:"id"`
	Symbol     string `yaml:"symbol" json:"symbol"`
	Name       string `yaml:"name" json:"name"`
	ISIN       string `yaml:"isin,omitempty" json:"isin,omitempty"`
	Exchange   string `yaml:"exchange" json:"exchange"`
	MIC        string `yaml:"mic" json:"mic"`
	Currency   string `yaml:"currency" json:"currency"`
	IssueType  string `yaml:"issue_type" json:"issue_type"`
	AssetClass string `yaml:"asset_class,omitempty" json:"asset_class,omitempty"`
	Sector     string `yaml:"sector,omitempty" json:"sector,omitempty"`
	Key        string `yaml:"key" json:"key"`
}

// Company is the broker's company page: dividend figures, ratios and the analyst consensus.
type Company struct {
	InstrumentID     string   `yaml:"instrument_id" json:"instrument_id"`
	Symbol           string   `yaml:"symbol" json:"symbol"`
	Name             string   `yaml:"name" json:"name"`
	ISIN             string   `yaml:"isin,omitempty" json:"isin,omitempty"`
	IssueType        string   `yaml:"issue_type" json:"issue_type"`
	Sector           string   `yaml:"sector,omitempty" json:"sector,omitempty"`
	Country          string   `yaml:"country,omitempty" json:"country,omitempty"`
	MarketCap        float64  `yaml:"market_cap,omitempty" json:"market_cap,omitempty"`
	Employees        float64  `yaml:"employees,omitempty" json:"employees,omitempty"`
	DividendYieldPct *float64 `yaml:"dividend_yield_pct,omitempty" json:"dividend_yield_pct,omitempty"` // nil: not stated
	TTMDividend      *float64 `yaml:"ttm_dividend,omitempty" json:"ttm_dividend,omitempty"`
	NextDividendDate string   `yaml:"next_dividend_date,omitempty" json:"next_dividend_date,omitempty"`
	ExDividendDate   string   `yaml:"ex_dividend_date,omitempty" json:"ex_dividend_date,omitempty"`
	PayoutRatioPct   float64  `yaml:"payout_ratio_pct,omitempty" json:"payout_ratio_pct,omitempty"`
	DivGrowth3YPct   float64  `yaml:"div_growth_3y_pct,omitempty" json:"div_growth_3y_pct,omitempty"`
	DivGrowth10YPct  float64  `yaml:"div_growth_10y_pct,omitempty" json:"div_growth_10y_pct,omitempty"`
	EPSGrowth3YPct   float64  `yaml:"eps_growth_3y_pct,omitempty" json:"eps_growth_3y_pct,omitempty"`
	PE               float64  `yaml:"pe,omitempty" json:"pe,omitempty"`
	DebtToEquityPct  float64  `yaml:"debt_to_equity_pct,omitempty" json:"debt_to_equity_pct,omitempty"`
	Beta             float64  `yaml:"beta,omitempty" json:"beta,omitempty"`
	TargetMean       float64  `yaml:"target_mean,omitempty" json:"target_mean,omitempty"`
	TargetLow        float64  `yaml:"target_low,omitempty" json:"target_low,omitempty"`
	TargetHigh       float64  `yaml:"target_high,omitempty" json:"target_high,omitempty"`
	TargetAnalysts   int      `yaml:"target_analysts,omitempty" json:"target_analysts,omitempty"`
	RatingMean       float64  `yaml:"rating_mean,omitempty" json:"rating_mean,omitempty"`
	RatingDate       string   `yaml:"rating_date,omitempty" json:"rating_date,omitempty"`
}

// Fund is the broker's fund page: policy, costs, yield, holdings and documents.
type Fund struct {
	InstrumentID       string   `yaml:"instrument_id" json:"instrument_id"`
	Symbol             string   `yaml:"symbol" json:"symbol"`
	Name               string   `yaml:"name" json:"name"`
	LegalName          string   `yaml:"legal_name,omitempty" json:"legal_name,omitempty"`
	ISIN               string   `yaml:"isin" json:"isin"`
	Manager            string   `yaml:"manager,omitempty" json:"manager,omitempty"`
	Domicile           string   `yaml:"domicile,omitempty" json:"domicile,omitempty"`
	Benchmark          string   `yaml:"benchmark,omitempty" json:"benchmark,omitempty"`
	BaseCurrency       string   `yaml:"base_currency,omitempty" json:"base_currency,omitempty"`
	ShareCurrency      string   `yaml:"share_currency,omitempty" json:"share_currency,omitempty"`
	DistributionPolicy string   `yaml:"distribution_policy" json:"distribution_policy"`
	TERPct             float64  `yaml:"ter_pct" json:"ter_pct"`
	YieldPct           *float64 `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	Dividend           *float64 `yaml:"dividend,omitempty" json:"dividend,omitempty"`
	DividendCurrency   string   `yaml:"dividend_currency,omitempty" json:"dividend_currency,omitempty"`
	AUM                float64  `yaml:"aum_millions,omitempty" json:"aum_millions,omitempty"`
	AUMCurrency        string   `yaml:"aum_currency,omitempty" json:"aum_currency,omitempty"`
	Return5YPct        float64  `yaml:"return_5y_pct,omitempty" json:"return_5y_pct,omitempty"`
	RiskScore          int      `yaml:"risk_score,omitempty" json:"risk_score,omitempty"`
	Launched           string   `yaml:"launched,omitempty" json:"launched,omitempty"`
	TopHoldings        []Weight `yaml:"top_holdings,omitempty" json:"top_holdings,omitempty"`
	Countries          []Weight `yaml:"countries,omitempty" json:"countries,omitempty"`
	// Industries are the broker's industry weights (e.g. "Consumer Essentials"), in percent.
	Industries []Weight `yaml:"industries,omitempty" json:"industries,omitempty"`
	// KIDURL is the PRIIPs KID, English first; Documents lists every fund document.
	KIDURL    string     `yaml:"kid_url,omitempty" json:"kid_url,omitempty"`
	Documents []Document `yaml:"documents,omitempty" json:"documents,omitempty"`
}

// Document is one fund document.
type Document struct {
	Type     string `yaml:"type" json:"type"`
	Language string `yaml:"language,omitempty" json:"language,omitempty"`
	Date     string `yaml:"date,omitempty" json:"date,omitempty"`
	URL      string `yaml:"url" json:"url"`
}

// Weight is one named share of a fund, in percent.
type Weight struct {
	Name string  `yaml:"name" json:"name"`
	Pct  float64 `yaml:"pct" json:"pct"`
}

// NewsItem is one headline the broker lists for an instrument. The broker's related-instrument ids
// are not carried: nothing reads them, and they cost 0.6–1.7 KB a name.
type NewsItem struct {
	ID        string `yaml:"id" json:"id"`
	Title     string `yaml:"title" json:"title"`
	Type      string `yaml:"type" json:"type"`
	Source    string `yaml:"source,omitempty" json:"source,omitempty"`
	Origin    string `yaml:"origin,omitempty" json:"origin,omitempty"`
	URL       string `yaml:"url,omitempty" json:"url,omitempty"`
	Published string `yaml:"published" json:"published"`
	Sentiment string `yaml:"sentiment,omitempty" json:"sentiment,omitempty"`
}

// EarningsCall is the latest earnings call the broker links.
type EarningsCall struct {
	InstrumentID string `yaml:"instrument_id" json:"instrument_id"`
	Period       string `yaml:"period" json:"period"`
	Year         int    `yaml:"year,omitempty" json:"year,omitempty"`
	Quarter      int    `yaml:"quarter,omitempty" json:"quarter,omitempty"`
	ReportTime   string `yaml:"report_time,omitempty" json:"report_time,omitempty"`
	AudioURL     string `yaml:"audio_url,omitempty" json:"audio_url,omitempty"`
	PDFURL       string `yaml:"pdf_url,omitempty" json:"pdf_url,omitempty"`
	Provider     string `yaml:"provider,omitempty" json:"provider,omitempty"`
}

// CorporateEvent is one dividend declaration or payment the broker lists.
type CorporateEvent struct {
	InstrumentID string  `yaml:"instrument_id" json:"instrument_id"`
	Type         string  `yaml:"type" json:"type"`
	Time         string  `yaml:"time" json:"time"`
	ExDate       string  `yaml:"ex_date,omitempty" json:"ex_date,omitempty"`
	PaymentDate  string  `yaml:"payment_date,omitempty" json:"payment_date,omitempty"`
	Announced    string  `yaml:"announced,omitempty" json:"announced,omitempty"`
	Amount       float64 `yaml:"amount,omitempty" json:"amount,omitempty"`
	Currency     string  `yaml:"currency,omitempty" json:"currency,omitempty"`
	Frequency    string  `yaml:"frequency,omitempty" json:"frequency,omitempty"`
}
