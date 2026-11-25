package rules

import (
	"math"
	"testing"
)

func p(v float64) *float64 { return &v }

func checkOf(t *testing.T, v Verdict, name string) Check {
	t.Helper()
	for _, c := range v.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check %s in %+v", name, v.Checks)
	return Check{}
}

func TestGroupsFollowTheProfile(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Input
		want string
	}{
		{"goat", Input{YieldPct: p(7), Growth5y: p(7), MarketCapUSDm: p(5000)}, GOAT},
		{"yielder", Input{YieldPct: p(6.5), Growth5y: p(2), MarketCapUSDm: p(500)}, Yielder},
		{"grower large", Input{YieldPct: p(3), Growth5y: p(12), MarketCapUSDm: p(50_000)}, Grower},
		{"stable grower", Input{YieldPct: p(3), Growth5y: p(5), IncreaseYears: 12, MarketCapUSDm: p(300_000)}, StableGrower},
		{"stable payer", Input{YieldPct: p(4), Growth5y: p(1), NoCutYears: 25, MarketCapUSDm: p(20_000)}, StablePayer},
		{"growth no dividend", Input{MarketCapUSDm: p(4_000_000)}, Growth},
		{"growth low yield", Input{YieldPct: p(0.8), Growth5y: p(3), MarketCapUSDm: p(100_000)}, Growth},
		{"no group", Input{YieldPct: p(4), Growth5y: p(3), MarketCapUSDm: p(100_000)}, "none"},
	} {
		if got := Evaluate(tc.in).Group; got != tc.want {
			t.Errorf("%s: group %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAGrowerBelowLargeCapIsNotAGrower(t *testing.T) {
	v := Evaluate(Input{YieldPct: p(3), Growth5y: p(15), MarketCapUSDm: p(3000)})
	if v.Group != "none" || len(v.GroupNotes) == 0 {
		t.Errorf("mid-cap grower = %q %v", v.Group, v.GroupNotes)
	}
}

func TestGroupGrowthFallsBackToThreeYears(t *testing.T) {
	v := Evaluate(Input{YieldPct: p(6.2), Growth3y: p(8)})
	if v.Group != GOAT || v.GroupBasis != "3y dividend CAGR (no 5y)" {
		t.Errorf("%q %q", v.Group, v.GroupBasis)
	}
}

func TestSafeDividendPassesAndFails(t *testing.T) {
	good := Input{Country: "US", YieldPct: p(4), Growth1y: p(3), Growth3y: p(4), Growth5y: p(5),
		PayoutPct: p(60), FCFPayoutPct: p(50), DebtToFCF: p(2), MarketCapUSDm: p(5000)}
	if v := Evaluate(good); v.Safe != Pass {
		t.Fatalf("good name = %s %+v", v.Safe, v.Checks)
	}
	cut := good
	cut.Growth1y = p(-5)
	if v := Evaluate(cut); v.Safe != Fail || checkOf(t, v, "growth_1y").Status != Fail {
		t.Errorf("a cut passed: %+v", v.Checks)
	}
	high := good
	high.PayoutPct = p(85)
	if v := Evaluate(high); checkOf(t, v, "payout").Status != Fail {
		t.Errorf("85%% payout passed")
	}
	debt := good
	debt.DebtToFCF = p(4)
	if v := Evaluate(debt); checkOf(t, v, "debt_to_fcf").Status != Fail {
		t.Errorf("Debt/FCF 4 passed on a mid cap")
	}
	debt.MarketCapUSDm = p(50_000)
	if v := Evaluate(debt); checkOf(t, v, "debt_to_fcf").Status != Skipped {
		t.Errorf("a large cap was not exempt from Debt/FCF")
	}
	uncovered := good
	uncovered.FCFPayoutPct = p(120)
	if v := Evaluate(uncovered); checkOf(t, v, "cover").Status != Fail {
		t.Errorf("cover 0.83x passed")
	}
}

func TestMissingDataNeedsALook(t *testing.T) {
	v := Evaluate(Input{YieldPct: p(5), Country: "GB"})
	if v.Safe != NeedsLook {
		t.Fatalf("safe = %s", v.Safe)
	}
	for _, c := range v.Checks {
		if c.Status == Pass || c.Status == Fail {
			t.Errorf("%s judged with no data: %s", c.Name, c.Status)
		}
	}
}

func TestAREITIsJudgedOnFFOOrOCF(t *testing.T) {
	in := Input{Country: "US", Sector: "Real Estate", Industry: "Equity Real Estate Investment Trusts (REITs)",
		YieldPct: p(5.5), Growth1y: p(2), Growth3y: p(2), Growth5y: p(3), PayoutPct: p(220),
		OCFPayoutPct: p(80), FCFPayoutPct: p(150), MarketCapUSDm: p(55_000)}
	v := Evaluate(in)
	if !v.REIT || checkOf(t, v, "payout").Status != Pass || checkOf(t, v, "cover").Status != Pass {
		t.Fatalf("REIT on OCF: %+v", v.Checks)
	}
	in.FFOPayoutPct = p(95)
	if c := checkOf(t, Evaluate(in), "payout"); c.Status != Fail || c.Note != "REIT, on EDGAR FFO" {
		t.Errorf("FFO payout 95: %+v", c)
	}
}

func TestBanksSkipCoverAndDebt(t *testing.T) {
	v := Evaluate(Input{Country: "GB", Sector: "Financials", Industry: "Banks", YieldPct: p(5),
		Growth1y: p(5), Growth3y: p(5), Growth5y: p(5), PayoutPct: p(45), MarketCapUSDm: p(5000)})
	if checkOf(t, v, "cover").Status != Skipped || checkOf(t, v, "debt_to_fcf").Status != Skipped || v.Safe != Pass {
		t.Errorf("bank: %s %+v", v.Safe, v.Checks)
	}
}

func TestWithholdingAndNetYieldAtTheBroker(t *testing.T) {
	for _, tc := range []struct {
		country string
		reit    bool
		rate    float64
		net     float64
	}{{"US", false, 25, 7.5}, {"CA", false, 22, 7.8}, {"ZZ", false, 0, 10}, {"GB", true, 18, 8.2}, {"GB", false, 0, 10}} {
		w := WithholdingFor(tc.country, "", tc.reit)
		if w == nil || w.RatePct != tc.rate {
			t.Fatalf("%s: %+v", tc.country, w)
		}
		if n := NetYield(p(10), w); n == nil || math.Abs(*n-tc.net) > 1e-9 {
			t.Errorf("%s net = %v", tc.country, n)
		}
	}
}

func TestCapBands(t *testing.T) {
	for v, want := range map[float64]string{250_000: Mega, 200_000: Mega, 15_000: Large, 2_000: Mid, 1_999: Small} {
		if got := CapBand(p(v)); got != want {
			t.Errorf("%v: %s want %s", v, got, want)
		}
	}
	if CapBand(nil) != "" {
		t.Error("unknown cap banded")
	}
}
