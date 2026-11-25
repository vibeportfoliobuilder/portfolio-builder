package rules

// reinvest.go - where each currency's dividends are reinvested: a currency's income buys the fund
// lines that trade in it (USD into the USD lines, EUR into the EUR lines); a currency the strategy
// lists under stock_reinvest_currencies goes back into the stocks bought in it instead (a currency
// with no suitable fund). An accumulating fund takes only its own currency. Income with no
// matching line is converted at the broker's FX cost into the distributing funds (else the paying lines).
// The Portfolio page's reinvestTargets mirrors this function; keep the two in step.

import (
	"slices"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// FXCostPct is the active broker's currency conversion cost (its pack's fx_cost_pct), charged on
// reinvested income that has to change currency.
func FXCostPct() float64 { return pack.Active().Broker.FXCostPct }

// Target is one line a currency's income buys: its share of that income and the FX cost paid.
type Target struct {
	Line      int     `json:"line"`
	Share     float64 `json:"share"`
	FXCostPct float64 `json:"fx_cost_pct"`
}

// ReinvestTargets lists the lines income in currency ccy buys, at their target weights. With no
// currency ("") it is the old pooled rule: every fund line, or every line when there is no fund.
func ReinvestTargets(lines []YoCLine, ccy string) []Target {
	ccy = strings.ToUpper(ccy)
	buy := func(l YoCLine) string { return strings.ToUpper(l.BuyCurrency) }
	hasFund := false
	for _, l := range lines {
		if l.Weight > 0 && l.Fund {
			hasFund = true
		}
	}
	pick := func(ok func(YoCLine) bool, fx bool) []Target {
		var tot float64
		for _, l := range lines {
			if l.Weight > 0 && ok(l) {
				tot += l.Weight
			}
		}
		if tot <= 0 {
			return nil
		}
		var out []Target
		for i, l := range lines {
			if l.Weight <= 0 || !ok(l) {
				continue
			}
			t := Target{Line: i, Share: l.Weight / tot}
			if fx && buy(l) != "" && buy(l) != ccy {
				t.FXCostPct = FXCostPct()
			}
			out = append(out, t)
		}
		return out
	}
	if ccy == "" {
		if hasFund {
			return pick(func(l YoCLine) bool { return l.Fund }, false)
		}
		return pick(func(YoCLine) bool { return true }, false)
	}
	stockCcy := StockReinvestCurrency(ccy)
	steps := []struct {
		ok func(YoCLine) bool
		fx bool
	}{
		{func(l YoCLine) bool { return stockCcy && !l.Fund && buy(l) == ccy }, false},
		{func(l YoCLine) bool { return l.Fund && buy(l) == ccy }, false},
		{func(l YoCLine) bool { return !hasFund && buy(l) == ccy }, false},
		{func(l YoCLine) bool { return l.Fund && !l.Acc }, true},
		{func(l YoCLine) bool { return !l.Acc }, true},
	}
	for _, s := range steps {
		if t := pick(s.ok, s.fx); t != nil {
			return t
		}
	}
	return nil
}

// StockReinvestCurrency reports a currency the strategy reinvests into its own stocks
// (stock_reinvest_currencies).
func StockReinvestCurrency(ccy string) bool {
	return slices.ContainsFunc(pack.Active().Strategy.StockReinvestCurrencies, func(c string) bool {
		return strings.EqualFold(strings.TrimSpace(c), ccy)
	})
}
