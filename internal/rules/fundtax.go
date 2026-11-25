package rules

import (
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/screen"
)

// FundDomicile is where a fund is set up, read off its ISIN prefix ("" when none).
func FundDomicile(isin string) string {
	if len(isin) < 2 {
		return ""
	}
	return strings.ToUpper(isin[:2])
}

// FundWithholding is what the active packs take from a fund's distribution, by the fund's
// domicile: the country pack's fund_withholding rate for that domicile (an Irish or Luxembourg
// UCITS fund usually pays gross), else the domicile's dividend rate (a US fund is a US payer).
// nil when neither states one: unknown, never a zero that would read as tax-free.
func FundWithholding(isin string) *Withholding {
	d := FundDomicile(isin)
	if d == "" {
		return nil
	}
	if r, ok := pack.Active().Country.FundWithholding[d]; ok {
		return &Withholding{Country: d, RatePct: r, Tier: screen.TierOf(r)}
	}
	return WithholdingFor(d, isin, false)
}
