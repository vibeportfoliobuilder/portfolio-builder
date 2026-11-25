package rules

// losspayout_test.go - a loss year's payout is "n/a (loss)" and judged on cash flow.

import (
	"strings"
	"testing"
)

func TestALossYearIsJudgedOnTheCashFlowPayout(t *testing.T) {
	in := Input{Country: "US", Sector: "Consumer Staples", YieldPct: p(6.9), Growth1y: p(2), Growth3y: p(3), Growth5y: p(4),
		PayoutPct: p(-146.95), FCFPayoutPct: p(85.05), OCFPayoutPct: p(63.61), DebtToFCF: p(5), MarketCapUSDm: p(25000)}
	c := checkOf(t, Evaluate(in), "payout")
	if c.Status != Fail || *c.Value != 85.05 || !strings.Contains(c.Note, PayoutLoss) || !strings.Contains(c.Limit, "free cash flow") {
		t.Errorf("GIS-like: %+v", c)
	}
	in.FCFPayoutPct = p(70)
	in.EarningsNote = "one-off loss quarter 2026-05-31"
	if c := checkOf(t, Evaluate(in), "payout"); c.Status != Pass || !strings.Contains(c.Note, "one-off loss quarter") {
		t.Errorf("FCF 70: %+v", c)
	}
	// Flagged loss with no ratio at all, and no positive cash-flow payout: needs a look, never a pass.
	in = Input{Country: "US", YieldPct: p(5), EarningsLoss: true, FCFPayoutPct: p(-20)}
	if c := checkOf(t, Evaluate(in), "payout"); c.Status != NeedsLook {
		t.Errorf("no cash payout: %+v", c)
	}
	in.OCFPayoutPct = p(50)
	if c := checkOf(t, Evaluate(in), "payout"); c.Status != Pass || !strings.Contains(c.Limit, "operating cash flow") {
		t.Errorf("OCF fallback: %+v", c)
	}
}
