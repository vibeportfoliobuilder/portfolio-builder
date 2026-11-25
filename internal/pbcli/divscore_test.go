package pbcli

import (
	"testing"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/divscore"
)

func fp(v float64) *float64 { return &v }

// ko is a dossier shaped like the old tests' safe payer, in the new file's terms.
func ko() (eulerpool.Dossier, Derived) {
	d := eulerpool.Dossier{}
	d.Ratios = eulerpool.Ratios{PayoutRatio: fp(35), FCFPayoutRatio: fp(38), DebtToEquity: fp(0.6), ROE: fp(30), ROIC: fp(16), OpMargin: fp(26)}
	d.Years = []eulerpool.Period{
		{FiscalYear: 2023, Statements: eulerpool.Statements{FCF: fp(7000)}},
		{FiscalYear: 2024, Statements: eulerpool.Statements{FCF: fp(8000)}},
		{FiscalYear: 2025, Statements: eulerpool.Statements{FCF: fp(9000), TotalDebt: fp(20000), DandA: fp(1000)}},
	}
	d.TTM = &eulerpool.Period{Statements: eulerpool.Statements{Revenue: fp(40000), FCF: fp(7200), OpIncome: fp(9000), NetDebt: fp(14000), TotalDebt: fp(20000)}}
	d.Dividends = &eulerpool.DividendHistory{TTMPerShare: fp(2), Yield: fp(2.5), PaymentYears: 30, IncreaseStreakYears: 28,
		ByFY: []eulerpool.FYTotal{{FiscalYear: 2023, Total: 1.8, FullyPaid: true}, {FiscalYear: 2024, Total: 1.9, FullyPaid: true}, {FiscalYear: 2025, Total: 2, FullyPaid: true}}}
	out := Derived{YieldPct: fp(2.5), DividendGrowth: map[string]*float64{"5y": fp(6)}}
	return d, out
}

var at = time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)

func TestScoreInputReadsTheDossier(t *testing.T) {
	d, out := ko()
	in := scoreInput(d, out, nil, nil, at)
	if !in.PaysDividend || in.NetDebtToEBITDA == nil || *in.NetDebtToEBITDA != 1.4 || in.FCFMargin == nil || *in.FCFMargin != 18 {
		t.Fatalf("input %+v", in)
	}
	if in.FCFCover == nil || *in.FCFCover < 2.6 || *in.FCFCover > 2.64 {
		t.Errorf("cover %v", in.FCFCover)
	}
	s := divscore.Compute(in)
	// Interest cover and the two recession criteria are not in the files: 87 points scoreable.
	if s.Scoreable != 87 || s.Safety == nil || s.Band != "Very Safe" {
		t.Errorf("scoreable %v safety %v %q", s.Scoreable, s.Safety, s.Band)
	}
}

// Ported from old: the first factor is a three-year average, and return on equity falls back to ROIC.
func TestTheFactorsAreAveragedAndFallBack(t *testing.T) {
	d, out := ko()
	d.Years[2].Statements.FCF = fp(-3480) // KO's working-capital swing
	in := scoreInput(d, out, nil, nil, at)
	if in.CashFlowToDebt == nil || *in.CashFlowToDebt <= 0 || in.CashFlowBasis != divscore.CashFlowThreeYearAverage {
		t.Errorf("cash flow to debt %v (%s)", in.CashFlowToDebt, in.CashFlowBasis)
	}
	d.Ratios.ROE = nil // MCD states none
	in = scoreInput(d, out, nil, nil, at)
	if in.ReturnBasis != divscore.ReturnOnInvestedCapital || in.ROE == nil || *in.ROE != 16 {
		t.Errorf("return %v (%s)", in.ROE, in.ReturnBasis)
	}
}

func TestACutYearIsReadFromFullyPaidYearsAndCaps(t *testing.T) {
	d, out := ko()
	d.Dividends.ByFY = []eulerpool.FYTotal{{FiscalYear: 2018, Total: 2, FullyPaid: true}, {FiscalYear: 2019, Total: 1, FullyPaid: true},
		{FiscalYear: 2020, Total: 1.2, FullyPaid: true}, {FiscalYear: 2026, Total: 0.5, FullyPaid: false}}
	in := scoreInput(d, out, nil, nil, at)
	if len(in.CutYears) != 1 || in.CutYears[0] != 2019 {
		t.Fatalf("cuts %v", in.CutYears)
	}
	if s := divscore.Compute(in); s.Safety == nil || *s.Safety != divscore.CapCutCeiling {
		t.Errorf("safety %v", s.Safety)
	}
	// A loss reads as the worst payout, not full marks.
	d, out = ko()
	d.Ratios.PayoutRatio = fp(-50)
	if in := scoreInput(d, out, nil, nil, at); in.PayoutEarnings == nil || *in.PayoutEarnings < 100 {
		t.Errorf("loss payout %v", in.PayoutEarnings)
	}
}
