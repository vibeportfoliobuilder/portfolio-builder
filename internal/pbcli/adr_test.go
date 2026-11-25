package pbcli

// adr_test.go - ADRs: issuer-country withholding and the cash-flow dividend.

import (
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

func TestAnADRIsTaxedAtTheIssuersCountry(t *testing.T) {
	// Made-up ISINs: a US-numbered receipt of a Taiwanese issuer, and a US company.
	if w := rules.WithholdingFor("TW", "US0000000051", false); w == nil || w.Country != "TW" || w.RatePct != 21 {
		t.Errorf("TW receipt: %+v", w)
	}
	if w := rules.WithholdingFor("US", "US0000000052", false); w == nil || w.RatePct != 25 {
		t.Errorf("US: %+v", w)
	}
}

func TestCashDividendStandsInForAMissingLedger(t *testing.T) {
	d := eulerpool.Dossier{Currency: "USD", PriceCurrency: "USD", Price: eulerpool.Price{Current: fp(353.10)},
		TTM:    &eulerpool.Period{Statements: eulerpool.Statements{DividendsPaid: fp(-16491.81)}},
		Shares: eulerpool.Shares{Outstanding: fp(5186.47), ShareCountTrend: []eulerpool.ShareCount{{Period: "2024-12-31", Shares: 5186}, {Period: "2025-12-31", Shares: 5186}}},
		Years: []eulerpool.Period{{FiscalYear: 2024, Statements: eulerpool.Statements{DividendsPaid: fp(12000)}},
			{FiscalYear: 2025, Statements: eulerpool.Statements{DividendsPaid: fp(15000)}}}}
	y, g, note := cashDividend(d)
	if y == nil || *y != 0.9 || g["1y"] == nil || *g["1y"] != 25 || !strings.Contains(note, "3.18 per share") {
		t.Errorf("%v %v %q", y, g, note)
	}
	d.PriceCurrency = "TWD"
	if y, _, _ := cashDividend(d); y != nil {
		t.Error("statements and price in different currencies must not give a yield")
	}
}
