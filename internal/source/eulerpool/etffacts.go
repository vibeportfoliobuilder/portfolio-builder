package eulerpool

// etffacts.go - a fund's facts for the Screen page and the look-through: the
// profile (TER, AUM, domicile) and every holding of 0.5% or more, two cached requests per ISIN.

import (
	"cmp"
	"context"
	"slices"
)

// MinLookThroughPct is the smallest holding the look-through reports.
const MinLookThroughPct = 0.5

// FundHolding is one holding with its identity, for matching it to a watched or liked name.
type FundHolding struct {
	Name   string  `yaml:"name" json:"name"`
	Symbol string  `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	ISIN   string  `yaml:"isin,omitempty" json:"isin,omitempty"`
	Pct    float64 `yaml:"pct" json:"pct"`
}

// ETFFacts is what the Screen page shows for a fund beyond the public screener.
type ETFFacts struct {
	TER         *float64      `yaml:"ter_pct,omitempty" json:"ter_pct,omitempty"`
	AUMMillions *float64      `yaml:"aum_millions,omitempty" json:"aum_millions,omitempty"`
	Domicile    string        `yaml:"domicile,omitempty" json:"domicile,omitempty"`
	Issuer      string        `yaml:"issuer,omitempty" json:"issuer,omitempty"`
	Holdings    []FundHolding `yaml:"holdings_over_half_pct,omitempty" json:"holdings_over_half_pct,omitempty"`
}

// Facts reads a fund's profile and holdings; a missing holdings list is not an error.
func (c *Client) Facts(ctx context.Context, isin string) (ETFFacts, error) {
	p, err := c.ETFProfile(ctx, isin)
	if err != nil {
		return ETFFacts{}, err
	}
	f := ETFFacts{TER: round(p.ExpenseRatio.Ptr(), 4), Domicile: p.Domicile, Issuer: p.Issuer}
	if p.AUM.OK {
		f.AUMMillions = round(ptr(p.AUM.V/1e6), 1)
	}
	hs, err := c.ETFHoldings(ctx, isin)
	if err == nil {
		f.Holdings = holdingsOver(hs, MinLookThroughPct)
	}
	return f, nil
}

func ptr(v float64) *float64 { return &v }

// holdingsOver keeps every holding of at least min percent, largest first.
func holdingsOver(hs []ETFHolding, min float64) []FundHolding {
	var out []FundHolding
	for _, h := range hs {
		if h.Percent.OK && h.Percent.V >= min {
			out = append(out, FundHolding{Name: cmp.Or(h.Name, h.Symbol), Symbol: h.Symbol, ISIN: h.ISIN, Pct: *round(&h.Percent.V, 2)})
		}
	}
	slices.SortStableFunc(out, func(a, b FundHolding) int { return cmp.Compare(b.Pct, a.Pct) })
	return out
}
