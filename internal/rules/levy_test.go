package rules

// levy_test.go - the country pack's home transaction levy: on top of the commission, judged in the
// home currency, with its exemption and cap, and never on a venue that already prices a tax.

import (
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack/packtest"
)

func TestTheHomeLevyCountsOnASmallTicket(t *testing.T) {
	// 500 XTS a month into one home-market stock: 3 XTS minimum commission (0.6%) + the 0.3% levy = 0.9%.
	a := Allocation{Monthly: 500, Lines: []Line{{Ticker: "HOMEA", Kind: KindStock, WeightPct: 100, YieldPct: p(5), Country: "ZZ", DividendCurrency: "XTS"}}}
	r := CheckPortfolio(a, homeFX)
	if r.Lines[0].FeePct == nil || *r.Lines[0].FeePct < 0.89 || *r.Lines[0].FeePct > 0.91 {
		t.Errorf("fee %v, want 0.9%% (minimum commission plus the levy)", r.Lines[0].FeePct)
	}
}

func TestTheLevyExemptionAndCap(t *testing.T) {
	s := packtest.Set()
	s.Broker.LevyNoExemption = false
	if got := s.Levy("home", 40_000); got != nil {
		t.Errorf("a buy at the exemption limit was taxed: %+v", got)
	}
	if got := s.Levy("home", 100_000); len(got) != 1 || got[0].Amount != 300 {
		t.Errorf("0.3%% of 100,000: %+v", got)
	}
	if got := s.Levy("home", 10_000_000); len(got) != 1 || got[0].Amount != 15_000 {
		t.Errorf("the cap: %+v", got)
	}
	if got := s.Levy("lse", 100_000); got != nil {
		t.Errorf("another market was taxed: %+v", got)
	}
}

func TestAVenueTaxIsNotChargedTwice(t *testing.T) {
	s := packtest.Set()
	m := s.Broker.Markets["home"]
	m.TaxPct, m.TaxLabel = 0.2, "venue tax 0.2%"
	s.Broker.Markets["home"] = m
	pack.Use(s)
	t.Cleanup(packtest.Activate)
	a := Allocation{Monthly: 10_000, Lines: []Line{{Ticker: "HOMEA", Kind: KindStock, WeightPct: 100, YieldPct: p(5), Country: "ZZ", DividendCurrency: "XTS"}}}
	r := CheckPortfolio(a, homeFX)
	// 0.15% commission + the venue's 0.2%; the 0.3% country levy is not added on top.
	if r.Lines[0].FeePct == nil || *r.Lines[0].FeePct < 0.34 || *r.Lines[0].FeePct > 0.36 {
		t.Errorf("fee %v, want 0.35%%", r.Lines[0].FeePct)
	}
}
