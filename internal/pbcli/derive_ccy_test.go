package pbcli

// derive_ccy_test.go - P/FV across currencies and the currency
// the broker credits.

import (
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPriceToFairValueConvertsTheFairValue(t *testing.T) {
	fx := fxTable{"XTS": 1, "USD": 325.9292, "CAD": 231.7096, "GBP": 436.2525}
	// CNQ: CAD price, USD fair value.
	p, note, ok := fx.priceToFairValue(68.5, "CAD", 45, "USD")
	if !ok || *p != 1.08 || !strings.Contains(note, "45 USD = 63.3 CAD at USD/XTS 325.9292 and CAD/XTS 231.7096") {
		t.Fatalf("CNQ %v %q", p, note)
	}
	// EBNK: USD price, GBX fair value.
	if p, _, ok := fx.priceToFairValue(17.5647, "USD", 1570, "GBX"); !ok || *p != 0.84 {
		t.Fatalf("EBNK %v", p)
	}
	if p, note, ok := fx.priceToFairValue(10, "CHF", 9, "USD"); ok || p != nil || !strings.Contains(note, "no USD/XTS or CHF/XTS rate") {
		t.Fatalf("no rate: %v %q", p, note)
	}
}

func TestCreditedCurrencyFollowsTheTradedLine(t *testing.T) {
	d := t.TempDir()
	e := &env{data: d}
	ly := broker.Stock{}
	ly.Instrument.ID, ly.Instrument.Currency, ly.Instrument.MIC = "i-cnq", "USD", "XNYS"
	out := Derived{DividendCurrency: "CAD"}
	e.creditedCurrency(&out, target{}, ly)
	if out.DividendCurrency != "USD" || out.DividendCurrencyDeclared != "CAD" || !strings.Contains(out.DividendCurrencyBasis, "XNYS line in USD") {
		t.Fatalf("listing: %+v", out)
	}
	// A credited dividend in the account activity wins.
	act := "as_of: \"2031-03-08\"\naccounts:\n- transactions:\n  - {type: DIVIDEND, time: \"2026-10-16T05:47:28Z\", currency: CAD, instrument_id: i-cnq, amount: 1}\n"
	if err := os.MkdirAll(filepath.Join(d, "portfolio"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "portfolio", "activity.yaml"), []byte(act), 0o644); err != nil {
		t.Fatal(err)
	}
	out = Derived{DividendCurrency: "CAD"}
	e.creditedCurrency(&out, target{}, ly)
	if out.DividendCurrency != "CAD" || out.DividendCurrencyDeclared != "" || !strings.Contains(out.DividendCurrencyBasis, "activity.yaml, dividend of 2026-10-16") {
		t.Fatalf("activity: %+v", out)
	}
}
