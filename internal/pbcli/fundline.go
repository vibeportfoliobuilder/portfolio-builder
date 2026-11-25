package pbcli

// fundline.go - the vendor ticker of the fund line the broker trades: the
// broker's symbol with the vendor's suffix for its venue (a fund on XLON gets .L, one on XAMS
// gets .AS). pb ep etf and pb ep returns ask Eulerpool for that line before the ISIN's default.

import (
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// vendorSuffix is Eulerpool's ticker suffix per venue (MIC); US venues take none.
var vendorSuffix = map[string]string{
	"XLON": ".L", "XAMS": ".AS", "XETR": ".DE", "XFRA": ".F", "XPAR": ".PA", "XMIL": ".MI", "XSWX": ".SW",
	"XBRU": ".BR", "XMAD": ".MC", "XWBO": ".VI", "XHEL": ".HE", "XSTO": ".ST", "XCSE": ".CO", "XOSL": ".OL",
	"XLIS": ".LS", "XDUB": ".IR", "XBUD": ".BD", "XNYS": "", "XNAS": "", "ARCX": "", "BATS": "",
}

// fundLine is the line the broker trades for a fund (vendor ticker and currency), from its
// broker.yaml, else the watch item; empty when the venue has no known suffix.
func (e *env) fundLine(t target) eulerpool.Line {
	sym, mic, ccy := "", "", ""
	var ly broker.ETF
	if ok, _ := e.readYAML(&ly, "etfs", t.Ticker, broker.File); ok {
		sym, mic, ccy = ly.Instrument.Symbol, cmpOr(ly.Instrument.MIC, ly.Instrument.Exchange), ly.Instrument.Currency
	}
	if t.Item != nil {
		sym, mic, ccy = cmpOr(sym, t.Item.Ticker), cmpOr(mic, cmpOr(t.Item.MIC, t.Item.Exchange)), cmpOr(ccy, t.Item.Currency)
	}
	suf, ok := vendorSuffix[strings.ToUpper(mic)]
	if sym == "" || !ok {
		return eulerpool.Line{}
	}
	return eulerpool.Line{Symbol: strings.ToUpper(sym) + suf, Currency: strings.ToUpper(ccy)}
}
