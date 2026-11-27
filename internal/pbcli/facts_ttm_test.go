package pbcli

import (
	"strings"
	"testing"
)

// A latest fiscal year without FCF (the vendor has not filled it yet) takes the TTM's FCF for the
// profitability test and the Rule of 40, and says so; with no TTM FCF either, both stay unavailable.
func TestFactsGatesFallBackToTheTTMFCF(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	yr := func(fy int, rev, op float64, fcf *float64) factYear {
		y := factYear{FY: fy, Revenue: f(rev), OpIncome: f(op), FCF: fcf, Cash: f(10), Debt: f(20)}
		y.OpMarginPct = pctOf(y.OpIncome, y.Revenue)
		y.FCFMarginPc = pctOf(y.FCF, y.Revenue)
		return y
	}
	all := []factYear{yr(2022, 100, 20, f(15)), yr(2023, 110, 22, f(16)), yr(2024, 120, 25, f(18)), yr(2025, 150, 30, nil)}
	ttm := factYear{Period: "TTM", PeriodEnd: "2026-03-31", Revenue: f(160), FCF: f(32), Source: "financial_data.yaml ttm.statements"}
	ttm.FCFMarginPc = pctOf(ttm.FCF, ttm.Revenue)

	g := gates(all, &ttm)
	r40 := g["gate2_rule_of_40"]
	if r40.Value == nil || *r40.Value != 45 || r40.Passed == nil || !*r40.Passed {
		t.Fatalf("rule of 40 = growth 25 + TTM FCF margin 20 = 45, passed: %+v", r40)
	}
	if r40.Parts["fcf_basis"] != "TTM 2026-03-31" || !strings.Contains(r40.Note, "TTM") || !strings.Contains(r40.Source, "ttm.statements") {
		t.Errorf("the TTM basis must be named: %+v", r40)
	}
	if p := g["gate1_profitable"]; p.Passed == nil || !*p.Passed || p.Parts["fcf_basis"] != "TTM 2026-03-31" {
		t.Errorf("profitable on the TTM's FCF: %+v", p)
	}

	g = gates(all, nil)
	if g["gate2_rule_of_40"].Passed != nil || !strings.Contains(g["gate2_rule_of_40"].Note, unavailable) {
		t.Errorf("no TTM: rule of 40 stays unavailable: %+v", g["gate2_rule_of_40"])
	}
}
