package pbcli

// derive_ccy.go - the currency the broker credits a stock's dividend in. The vendor names the
// declared currency (CAD for a Canadian company); a broker whose pack says credits_line_currency
// credits the currency of the line it trades (USD for a Canadian company's New York line). A
// credited dividend in the account's activity (data/portfolio/activity.yaml) is the direct source.

import (
	"fmt"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// creditedCurrency sets dividend_currency to what the broker credits: a DIVIDEND row of the account
// activity for this instrument, else (credits_line_currency) the traded line's currency when it
// differs from the declared one.
func (e *env) creditedCurrency(out *Derived, t target, ly broker.Stock) {
	declared := out.DividendCurrency
	if declared == "" {
		return
	}
	id := ly.Instrument.ID
	if id == "" && t.Item != nil {
		id = t.Item.InstrumentID
	}
	b := pack.Active().Broker
	if c, when := e.activityDividendCurrency(id); c != "" {
		if c != declared {
			out.DividendCurrencyDeclared = declared
		}
		out.DividendCurrency = c
		out.DividendCurrencyBasis = fmt.Sprintf("credited in %s by %s (data/portfolio/activity.yaml, dividend of %s)", c, b.Name, when)
		return
	}
	if !b.CreditsLineCurrency {
		return
	}
	line := normCcy(ly.Instrument.Currency)
	venue := ly.Instrument.MIC
	if line == "" && t.Item != nil {
		line, venue = normCcy(t.Item.Currency), firstNonEmpty(t.Item.MIC, t.Item.Exchange)
	}
	if line == "" || line == declared {
		return
	}
	out.DividendCurrencyDeclared = declared
	out.DividendCurrency = line
	out.DividendCurrencyBasis = fmt.Sprintf("declared in %s; %s trades the %s line in %s and credits the line's currency (its broker pack: credits_line_currency); no dividend from it in the account activity yet",
		declared, b.Name, cmpOr(venue, "listed"), line)
}

// activityDividendCurrency is the currency of the newest DIVIDEND row for the instrument in
// data/portfolio/activity.yaml, and its date; "" without one. Amounts are never read.
func (e *env) activityDividendCurrency(instrumentID string) (string, string) {
	if instrumentID == "" {
		return "", ""
	}
	var doc struct {
		Accounts []struct {
			Transactions []struct {
				Type         string `yaml:"type"`
				Time         string `yaml:"time"`
				Currency     string `yaml:"currency"`
				InstrumentID string `yaml:"instrument_id"`
			} `yaml:"transactions"`
		} `yaml:"accounts"`
	}
	if ok, err := e.readYAML(&doc, "portfolio", "activity.yaml"); !ok || err != nil {
		return "", ""
	}
	best, when := "", ""
	for _, a := range doc.Accounts {
		for _, tx := range a.Transactions {
			if strings.EqualFold(tx.Type, "DIVIDEND") && tx.InstrumentID == instrumentID && tx.Currency != "" && tx.Time > when {
				best, when = normCcy(tx.Currency), tx.Time
			}
		}
	}
	return best, day10s(when)
}

func day10s(s string) string { return s[:min(10, len(s))] }

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
