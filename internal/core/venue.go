package core

// venue.go - what a venue code in the universe file means, in the kernel because
// four readers need it and none is above the others.

import "strings"

type VenueType int

const (
	LitPrimary VenueType = iota
	MTF
	RFQ
)

type Venue struct {
	Name         string
	Country      string
	Currency     string
	Type         VenueType
	Pence        bool
	AVSuffix     string
	HistoryVenue string // primary lit venue used for the SMART pin fallback
}

// ExchangeMap is what a venue code means, and these fields are only the DEFAULTS
// a row may omit. Every TRWB* entry came off a real contract; keep no unused venue.
var ExchangeMap = map[string]Venue{
	"EBS": {"SIX Swiss Exchange", "CH", "CHF", LitPrimary, false, ".SW", "EBS"},
	// IBIS carries the GERMAN-domiciled lines and IBIS2 the Irish UCITS ones; a
	// contract answers to one of the two and not to the other.
	"IBIS":      {"Xetra (Deutsche Boerse)", "DE", "EUR", LitPrimary, false, ".DE", "IBIS"},
	"IBIS2":     {"Xetra (Deutsche Boerse) ETF segment", "DE", "EUR", LitPrimary, false, ".DE", "IBIS2"},
	"LSE":       {"London Stock Exchange", "UK", "GBP", LitPrimary, true, ".LON", "LSE"},
	"LSEETF":    {"LSE ETF segment", "UK", "USD", LitPrimary, true, ".LON", "LSEETF"},
	"TRQXUK":    {"Turquoise Europe UK", "UK", "GBP", MTF, true, ".LON", "LSEETF"},
	"TRQXCH":    {"Turquoise Europe Switzerland", "CH", "CHF", MTF, false, ".SW", "EBS"},
	"AEB":       {"Euronext Amsterdam", "NL", "EUR", LitPrimary, false, ".AS", "AEB"},
	"SBF":       {"Euronext Paris", "FR", "EUR", LitPrimary, false, ".PA", "SBF"},
	"BVME.ETF":  {"Borsa Italiana ETFplus segment", "IT", "EUR", LitPrimary, false, ".MI", "BVME.ETF"},
	"SWB2":      {"Boerse Stuttgart ETF segment", "DE", "EUR", LitPrimary, false, ".SG", "SWB2"},
	"VSE":       {"Vienna Stock Exchange", "AT", "EUR", LitPrimary, false, ".VI", "VSE"},
	"AQEUEN":    {"Aquis Exchange Europe (Euronext)", "EU", "EUR", MTF, false, ".AS", "AEB"},
	"AQXECH":    {"Aquis Exchange PLC - Switzerland", "CH", "CHF", MTF, false, ".SW", "EBS"},
	"AQXEUK":    {"Aquis Exchange PLC - UK", "UK", "GBP", MTF, true, ".LON", "LSEETF"},
	"BATECH":    {"Cboe Europe BXE - Switzerland", "CH", "CHF", MTF, false, ".SW", "EBS"},
	"CHIXCH":    {"Cboe Europe CXE - Switzerland", "CH", "CHF", MTF, false, ".SW", "EBS"},
	"TRWBUKETF": {"Tradeweb UK ETF", "UK", "GBP", RFQ, true, ".LON", "LSEETF"},
	"TRWBCH":    {"Tradeweb Switzerland", "CH", "CHF", RFQ, false, ".SW", "EBS"},
	"TRWBEN":    {"Tradeweb EU (Euronext)", "EU", "EUR", RFQ, false, ".AS", "AEB"},
}

// countryNames maps a country's English name, lower-cased, to its ISO 3166 alpha-2: one table for
// every vendor that states a name. Fixed, not fuzzy - a fuzzy match turns an unknown name into a neighbour.
var countryNames = map[string]string{
	"australia": "AU", "austria": "AT", "bahamas": "BS", "belgium": "BE", "bermuda": "BM",
	"canada": "CA", "cayman islands": "KY", "cyprus": "CY", "czech republic": "CZ", "czechia": "CZ",
	"denmark": "DK", "estonia": "EE", "finland": "FI", "france": "FR", "germany": "DE",
	"gibraltar": "GI", "great britain": "GB", "greece": "GR", "guernsey": "GG", "hong kong": "HK",
	"hungary": "HU", "ireland": "IE", "isle of man": "IM", "israel": "IL", "italy": "IT",
	"japan": "JP", "jersey": "JE", "latvia": "LV", "liechtenstein": "LI", "lithuania": "LT",
	"luxembourg": "LU", "macao": "MO", "malta": "MT", "marshall islands": "MH", "mauritius": "MU",
	"mexico": "MX", "monaco": "MC", "netherlands": "NL", "norway": "NO", "peru": "PE", "poland": "PL",
	"portugal": "PT", "romania": "RO", "singapore": "SG", "slovenia": "SI", "south africa": "ZA",
	"spain": "ES", "sweden": "SE", "switzerland": "CH", "taiwan": "TW", "the bahamas": "BS",
	"united arab emirates": "AE", "united kingdom": "GB", "united states": "US",
	"united states of america": "US", "usa": "US", "virgin islands (british)": "VG",
}

// CountryCode is the ISO alpha-2 of a country name in any case, "" for one the table does not hold:
// the broker's fund domicile and the stock screener's country column both read it.
func CountryCode(name string) string {
	return countryNames[strings.ToLower(strings.TrimSpace(name))]
}

// MICCountry is the country a MIC names. Matching at COUNTRY level is deliberate:
// the FIRDS extract keeps one row per ISIN, whichever venue published first.
var MICCountry = map[string]string{
	// Germany. XETA and XETU are Xetra's two segments, FRAA/FRAU Frankfurt's,
	// HAMN Hamburg's - all four appear in this universe's own extract.
	"XETR": "DE", "XETA": "DE", "XETU": "DE",
	"FRAA": "DE", "FRAU": "DE", "XFRA": "DE", "HAMN": "DE", "XSTU": "DE", "XMUN": "DE",
	// Italy. ETFP is Borsa Italiana's ETFplus segment.
	"ETFP": "IT", "MTAA": "IT", "XMIL": "IT",
	// Euronext. XMSM is Euronext Dublin's main securities market, which is where
	// an Irish-domiciled fund's own register entry lands.
	"XPAR": "FR", "XAMS": "NL", "XAMC": "NL", "XBRU": "BE", "XLIS": "PT",
	"XMSM": "IE", "XDUB": "IE",
	// UK, Switzerland and the Nordics.
	"XLON": "UK", "XLOD": "UK", "AQXE": "UK", "TRQX": "UK", "BATE": "UK", "CHIX": "UK",
	"XSWX": "CH", "XQMH": "CH", "XVTX": "CH",
	"XSTO": "SE", "XCSE": "DK", "XHEL": "FI", "XOSL": "NO",
	"XWBO": "AT",
	// Tradeweb's European RFQ venue, which is where three of this universe's
	// register rows come from.
	"TWEM": "NL",
}

// micStockVenue is a broker's code for the venue a company row names by MIC, as a screener does.
// BUX, SBF, AEB, NASDAQ, ISED, PRA, WSE and TSE were each named by a broker's resolve.
var micStockVenue = map[string]string{
	"XNYS": "NYSE", "XNAS": "NASDAQ", "XASE": "AMEX", "XTSE": "TSE",
	"XBUD": "BUX", "XPRA": "PRA", "XWAR": "WSE", "XWBO": "VSE",
	"XPAR": "SBF", "XAMS": "AEB", "XBRU": "ENEXT.BE", "XLIS": "BVL", "XDUB": "ISED",
	"XETR": "IBIS", "XFRA": "FWB", "XMIL": "BVME", "XMAD": "BM", "BMEX": "BM", "XLON": "LSE", "XSWX": "EBS",
	"XHEL": "HEX", "XSTO": "SFB", "XCSE": "CPH", "XOSL": "OSE",
}

// StockVenue is the broker's venue code for a company row's exchange: a MIC is
// translated, and anything else comes back upper-cased as it was.
func StockVenue(exchange string) string {
	e := strings.ToUpper(strings.TrimSpace(exchange))
	if v, ok := micStockVenue[e]; ok {
		return v
	}
	return e
}

// usVenues are the broker's codes for the market a bare symbol means to the SEC's
// ticker map, Yahoo, stockanalysis and Finnhub.
var usVenues = map[string]bool{
	"NYSE": true, "NASDAQ": true, "NASDAQ.NMS": true, "NASDAQ.SCM": true,
	"AMEX": true, "ARCA": true, "BATS": true, "PINK": true, "OTC": true,
}

// USVenue reports a US market, a MIC or the broker's own code; "" is not one.
func USVenue(exchange string) bool { return usVenues[StockVenue(exchange)] }

// ForeignVenue reports a stated venue outside the US, where a bare-symbol origin
// answers for whichever US company holds that symbol. "" states nothing.
func ForeignVenue(exchange string) bool {
	return strings.TrimSpace(exchange) != "" && !USVenue(exchange)
}

// SameCountryVenue reports whether a MIC and a venue code name one country.
// Three answers: ok is false when either side is unknown, which is "cannot tell".
func SameCountryVenue(mic, exchange string) (same, ok bool) {
	mc, hasMIC := MICCountry[strings.ToUpper(strings.TrimSpace(mic))]
	v, hasVenue := ExchangeMap[strings.ToUpper(strings.TrimSpace(exchange))]
	if !hasMIC || !hasVenue || mc == "" || v.Country == "" {
		return false, false
	}
	return mc == v.Country, true
}
