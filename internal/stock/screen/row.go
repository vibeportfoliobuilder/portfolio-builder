package screen

// row.go - the identity of one listing, as much as the withholding lookup reads.

// Row is one listing. Payer is the country a receipt's dividend is sourced in,
// stated by hand where the ISIN says US.
type Row struct {
	Ticker   string `json:"ticker,omitempty"`
	Name     string `json:"name,omitempty"`
	Currency string `json:"currency,omitempty"`
	Exchange string `json:"exchange,omitempty"`
	// Type is the listing's classifier: COMMON, ETF, ADR, REIT, NY REG SHRS.
	Type    string `json:"type,omitempty"`
	ISIN    string `json:"isin,omitempty"`
	Country string `json:"country,omitempty"`
	Payer   string `json:"-"`
}
