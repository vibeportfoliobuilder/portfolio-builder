package pbcli

// packs.go - pb packs: which country and broker packs are active, and the built-in ones.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

func packsCmd(e *env) *cobra.Command {
	var brief bool
	c := &cobra.Command{
		Use:   "packs",
		Short: "Show the active country and broker packs (data/config.yaml) and the built-in ones",
		Long: `A country pack is where you live: home currency, inflation target, the withholding a dividend
suffers at source, a transaction tax on your home market, the bonds the switch to income buys.
A broker pack is where you buy: fees per market, FX cost, where its withholding differs.
It also prints your strategy (data/profile/strategy.yaml) and strategy_unset: the keys you have not
set, whose rules are skipped.
--brief prints only what the analysts judge by: the active names and home currency, the withholding
tables, the transaction taxes, a fee summary and the strategy (no sources, no market_by_country, no
built-in pack lists).
data/config.yaml names them; data/packs/country/<name>.yaml and data/packs/broker/<name>.yaml
are read first, else a built-in pack. With no config.yaml the made-up examples are used.
Example data/config.yaml:
  country: exampleland
  broker: examplebroker
  language: en
  pb packs --brief`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			a := pack.Active()
			if brief {
				return e.printJSON(packsBrief(a))
			}
			return e.printJSON(map[string]any{
				"active":           map[string]string{"country": a.Country.Name, "currency": a.Country.Currency, "broker": a.Broker.Name, "language": a.Language},
				"built_in_country": pack.Names("country"),
				"built_in_broker":  pack.Names("broker"),
				"country":          a.Country,
				"broker":           a.Broker,
				"strategy":         a.Strategy,
				"strategy_unset":   a.Strategy.Unset(),
			})
		},
	}
	c.Flags().BoolVar(&brief, "brief", false, "only the active names, currency, withholding, taxes, fees and strategy")
	return c
}

// packsBrief is the part of the active packs the analysts read: everything a rule or a cost needs,
// without the sources and the routing tables.
func packsBrief(a pack.Set) map[string]any {
	c, b := a.Country, a.Broker
	wh := map[string]any{"by_country_pct": c.Withholding}
	if len(c.REITWithholding) > 0 {
		wh["reit_pct"] = c.REITWithholding
	}
	if len(c.FundWithholding) > 0 {
		wh["fund_domicile_pct"] = c.FundWithholding
	}
	if len(c.Unrated) > 0 {
		wh["unrated"] = c.Unrated
	}
	if len(b.WithholdingOverrides) > 0 {
		wh["broker_overrides_pct"] = b.WithholdingOverrides
	}
	if len(b.REITWithholding) > 0 {
		wh["broker_reit_pct"] = b.REITWithholding
	}
	tax := map[string]any{}
	if c.TransactionTax != nil {
		tax["home_market"] = c.TransactionTax
	}
	if b.LevyNoExemption {
		tax["levy_no_exemption"] = true
	}
	fees := map[string]any{"fx_cost_pct": b.FXCostPct, "default_market": b.DefaultMarket, "default_etf_market": b.DefaultETFMarket}
	if b.CreditsLineCurrency {
		fees["credits_line_currency"] = true
	}
	if len(b.Markets) > 0 {
		ms := map[string]string{}
		venue := map[string]string{}
		for k, m := range b.Markets {
			ms[k] = marketSummary(m)
			if m.TaxPct > 0 {
				venue[k] = m.TaxLabel
				if venue[k] == "" {
					venue[k] = fmt.Sprintf("transaction tax %g%%", m.TaxPct)
				}
			}
		}
		fees["markets"] = ms
		if len(venue) > 0 {
			tax["venue"] = venue
		}
	} else {
		// A fee model prices the orders: show what one order of 1,000 costs in the default markets.
		q := map[string]string{}
		for _, m := range []string{b.DefaultMarket, b.DefaultETFMarket} {
			if m == "" {
				continue
			}
			cur, _ := b.Currency(m)
			q[m] = quoteSummary(b.Quote(m, 1000), cur)
		}
		fees["fee_model_quote_1000"] = q
	}
	return map[string]any{
		// active.currency is the home currency.
		"active":          map[string]string{"country": c.Name, "currency": c.Currency, "broker": b.Name, "language": a.Language},
		"withholding":     wh,
		"transaction_tax": tax,
		"fees":            fees,
		"strategy":        a.Strategy,
		"strategy_unset":  a.Strategy.Unset(),
	}
}

// marketSummary is one venue's price in a line: "XETRA EUR 1 + 0.1% (min 2, max 20)".
func marketSummary(m pack.Market) string {
	if m.Offered != nil && !*m.Offered {
		return m.Name + ": not offered"
	}
	s := fmt.Sprintf("%s %s %g", m.Name, m.Currency, m.Fixed)
	if m.PctFee > 0 {
		s += fmt.Sprintf(" + %g%%", m.PctFee)
	}
	var lim []string
	if m.Min > 0 {
		lim = append(lim, fmt.Sprintf("min %g", m.Min))
	}
	if m.Max > 0 {
		lim = append(lim, fmt.Sprintf("max %g", m.Max))
	}
	if len(lim) > 0 {
		s += " (" + strings.Join(lim, ", ") + ")"
	}
	if m.Kind != "" {
		s += " [" + m.Kind + "]"
	}
	return s
}

// quoteSummary is a quote in a line: "2.5 USD (commission 1.5; FX 1)", or why it cannot be bought.
func quoteSummary(q pack.Quote, cur string) string {
	if !q.Offered {
		return "not offered: " + q.Why
	}
	parts := make([]string, 0, len(q.Parts))
	for _, p := range q.Parts {
		parts = append(parts, fmt.Sprintf("%s %g", p.Label, p.Amount))
	}
	sort.Strings(parts)
	return strings.TrimSpace(fmt.Sprintf("%g %s (%s)", q.Total, cur, strings.Join(parts, "; ")))
}
