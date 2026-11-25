package rules

import "testing"

// The strategy has no starting-yield target: the figure is still
// reported, but no rule passes or fails on it.
func TestTheStartYieldIsReportedWithoutARule(t *testing.T) {
	r := CheckPortfolio(sample(), homeFX)
	for _, x := range r.Rules {
		if x.Name == "start_yield" {
			t.Fatalf("start_yield is still a rule: %+v", x)
		}
	}
	if r.StartYieldPct == nil || r.StartNetYieldPct == nil || *r.StartNetYieldPct >= *r.StartYieldPct {
		t.Errorf("start yield figures: %v %v", r.StartYieldPct, r.StartNetYieldPct)
	}
}
