package pbcli

// derive_ledger_test.go - a ledger that missed a split, streaks the
// ledger cannot close and specials out of the growth, on hand-built dossiers.

import (
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// KCR: 2.25 in the ledger against the broker's payout 44.67% × EPS 1.61 = 0.72: a 3:1 split.
func TestUnsplitLedgerFromBrokerPayout(t *testing.T) {
	ppy := 1.0
	mk := func() eulerpool.Dossier {
		return eulerpool.Dossier{Price: eulerpool.Price{Current: fp(28.02)}, Shares: eulerpool.Shares{Outstanding: fp(237.66)},
			TTM: &eulerpool.Period{Statements: eulerpool.Statements{EPSDiluted: fp(1.61), NetIncome: fp(383.7)}},
			Dividends: &eulerpool.DividendHistory{PaymentsPerYear: &ppy, TTMPerShare: fp(2.25), LastExDate: "2026-03-27",
				ByFY: []eulerpool.FYTotal{{FiscalYear: 2024, Total: 0.45, Count: 1, FullyPaid: true}, {FiscalYear: 2025, Total: 0.55, Count: 1, FullyPaid: true},
					{FiscalYear: 2026, Total: 2.25, Count: 1, FullyPaid: true}},
				Payments: []eulerpool.Payment{{ExDate: "2024-03-28", DPS: 0.45}, {ExDate: "2025-03-28", DPS: 0.55}, {ExDate: "2026-03-27", DPS: 2.25}}}}
	}
	d := mk()
	note := unsplitLedger(&d, 44.6728)
	if !strings.Contains(note, "3-for-1") || *d.Dividends.TTMPerShare != 0.75 || *d.Dividends.Yield != 2.68 {
		t.Fatalf("note %q ttm %v", note, *d.Dividends.TTMPerShare)
	}
	// A real raise that the broker's payout agrees with is left alone.
	d = mk()
	if n := unsplitLedger(&d, 140); n != "" || *d.Dividends.TTMPerShare != 2.25 {
		t.Fatalf("agreeing payout: %q", n)
	}
	// No factor fits: noted for a look, nothing changed.
	d = mk()
	if n := unsplitLedger(&d, 20); !strings.Contains(n, "needs a look") || *d.Dividends.TTMPerShare != 2.25 {
		t.Fatalf("no factor: %q", n)
	}
}

// ENB: a payment slipped from FY2016 into FY2017 (3 then 5 payments), so the raise streak stops at 8;
// BNS: FY2023 holds 6 payments. Both are "at least"; a dividend's first, short year is its start.
func TestStreakFloors(t *testing.T) {
	ppy := 4.0
	y := func(fy int, total float64, n int, full bool) eulerpool.FYTotal {
		return eulerpool.FYTotal{FiscalYear: fy, Total: total, Count: n, FullyPaid: full}
	}
	h := &eulerpool.DividendHistory{PaymentsPerYear: &ppy, ByFY: []eulerpool.FYTotal{y(2014, 1.515, 4, true), y(2015, 1.925, 4, true), y(2016, 1.59, 3, false),
		y(2017, 3.084, 5, true), y(2018, 2.751, 4, true), y(2019, 3.024, 4, true), y(2020, 3.265, 4, true)}, IncreaseStreakYears: 2, NoCutYears: 2, PaymentYears: 3}
	fl := eulerpool.StreakFloors(h)
	if !strings.Contains(fl["increase"], "FY2017 with 5 payments") || fl["payment"] == "" {
		t.Fatalf("ENB floors %v", fl)
	}
	// A dividend that starts with one payment in its first year is closed at the start.
	h = &eulerpool.DividendHistory{PaymentsPerYear: &ppy, ByFY: []eulerpool.FYTotal{y(2021, 0, 0, false), y(2022, 0.125, 1, true),
		y(2023, 0.5, 4, true), y(2024, 0.6, 4, true)}, IncreaseStreakYears: 2, NoCutYears: 2, PaymentYears: 3}
	if fl := eulerpool.StreakFloors(h); fl != nil {
		t.Fatalf("initiation: %v", fl)
	}
	// A ledger that begins on a fully paid year cannot close a run that reaches it.
	h = &eulerpool.DividendHistory{PaymentsPerYear: &ppy, ByFY: []eulerpool.FYTotal{y(1990, 0.5, 4, true), y(1991, 0.6, 4, true)},
		IncreaseStreakYears: 1, NoCutYears: 1, PaymentYears: 2}
	if fl := eulerpool.StreakFloors(h); len(fl) != 3 || !strings.Contains(fl["increase"], "start of the ledger, FY1990") {
		t.Fatalf("ledger start: %v", fl)
	}
}

// ACHP: the 2024 special in FY2025 made the 1y growth −32.4%; out of the totals it is a raise.
func TestDropSpecialsFromGrowth(t *testing.T) {
	ppy := 4.0
	d := eulerpool.Dossier{Years: []eulerpool.Period{{FiscalYear: 2026, PeriodEnd: "2026-01-25"}},
		Dividends: &eulerpool.DividendHistory{PaymentsPerYear: &ppy,
			ByFY: []eulerpool.FYTotal{{FiscalYear: 2024, Total: 0.016, Count: 4, FullyPaid: true}, {FiscalYear: 2025, Total: 0.074, Count: 5, Normalised: fp(0.0592), FullyPaid: true},
				{FiscalYear: 2026, Total: 0.04, Count: 4, FullyPaid: true}},
			Payments: []eulerpool.Payment{{ExDate: "2024-03-05", DPS: 0.004}, {ExDate: "2024-05-29", DPS: 0.04}, {ExDate: "2024-06-11", DPS: 0.01}}}}
	note := eulerpool.DropSpecials(&d, []string{"2024-05-29"})
	h := d.Dividends
	if !strings.Contains(note, "2024-05-29") || h.ByFY[1].Total != 0.034 || h.ByFY[1].Count != 4 || h.CAGR1y == nil || *h.CAGR1y != 17.65 {
		t.Fatalf("note %q fy %+v 1y %v", note, h.ByFY[1], h.CAGR1y)
	}
}
