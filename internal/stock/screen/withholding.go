package screen

// withholding.go - where a dividend is sourced, what that country withholds (the active
// country and broker packs state the rates) and the tier a reader picks by.

import (
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// The tiers, banded by what the source country keeps. S is the only one that
// withholds nothing, and C is what a terminated treaty reads as.
const (
	TierS = "S"
	TierA = "A"
	TierB = "B"
	TierC = "C"
)

// Tax is the withholding one row suffers at source, at the active packs' rate.
type Tax struct {
	// Country is the alpha-2 the rate was read for, so a tier is checkable
	// against the table rather than taken on trust.
	Country string  `json:"country"`
	Rate    float64 `json:"rate"`
	Tier    string  `json:"tier"`
}

// TaxOf is the withholding for one row at the active broker, nil where no country resolves: a
// rate nobody stated is absent, and a zero there would read as tax-free. reit marks a REIT,
// whose rate can differ (a UK REIT's property-income distribution).
func TaxOf(r Row, reit bool) *Tax {
	code := TaxCountry(r)
	rate, ok := pack.Active().Rate(code, reit)
	if !ok {
		return nil
	}
	return &Tax{Country: code, Rate: rate, Tier: TierOf(rate)}
}

// TaxCountry is where a dividend is SOURCED: the ISIN's own prefix first, because
// that is the country the issuer is in and a listing venue is not.
func TaxCountry(r Row) string {
	if len(r.ISIN) >= 2 {
		code := strings.ToUpper(r.ISIN[:2])
		if alias, ok := isinAliases[code]; ok {
			code = alias
		}
		if code == "US" && (r.Payer != "" || receipt(r)) {
			return receiptCountry(r)
		}
		if rated(code) || issuerCountry(code) {
			return code
		}
	}
	c := strings.TrimSpace(r.Country)
	if len(c) == 2 {
		return strings.ToUpper(c)
	}
	return core.CountryCode(c)
}

// receipt reports a line numbered in the US for a foreign payer: a New York registry
// share (Philips, Aegon) or a depositary receipt (BP), as the row's own type says.
func receipt(r Row) bool {
	t := strings.TrimSpace(r.Type)
	return strings.EqualFold(t, "NY REG SHRS") || strings.EqualFold(t, "ADR")
}

// receiptCountry is where a receipt is sourced: the universe's word, else a non-US country
// on an ADR row. A registry share's country is the venue's, so it stays unrated as before.
func receiptCountry(r Row) string {
	if p := strings.TrimSpace(r.Payer); p != "" {
		return strings.ToUpper(p)
	}
	if !strings.EqualFold(strings.TrimSpace(r.Type), "ADR") {
		return ""
	}
	c := strings.ToUpper(strings.TrimSpace(r.Country))
	if len(c) != 2 {
		c = core.CountryCode(r.Country)
	}
	if c == "US" {
		return ""
	}
	return c
}

// StatePayers fills each row's payer from the universe's hand-kept countries, by ticker.
func StatePayers(rows []Row, payers map[string]string) {
	for i := range rows {
		rows[i].Payer = payers[strings.ToUpper(strings.TrimSpace(rows[i].Ticker))]
	}
}

// isinAliases are ISIN prefixes that outlived their country: Czech securities are
// still numbered under Czechoslovakia's CS.
var isinAliases = map[string]string{"CS": "CZ"}

// issuerCountry reports an ISIN prefix that names a country, which outranks the
// venue even unrated: XS, the other X and Q codes and EU name no issuer's country.
func issuerCountry(code string) bool {
	if len(code) != 2 || code == "EU" || code[0] == 'X' || code[0] == 'Q' {
		return false
	}
	return code[0] >= 'A' && code[0] <= 'Z' && code[1] >= 'A' && code[1] <= 'Z'
}

// rated reports whether the active packs state a rate for a code.
func rated(code string) bool { return pack.Active().Rated(code) }

// TierOf bands a rate: S withholds nothing, A under 15, B under 30, and C is the
// 30-and-over that nothing on file reduces.
func TierOf(rate float64) string {
	switch {
	case rate <= 0:
		return TierS
	case rate < 15:
		return TierA
	case rate < 30:
		return TierB
	default:
		return TierC
	}
}
