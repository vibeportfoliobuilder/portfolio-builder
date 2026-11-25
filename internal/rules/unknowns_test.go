package rules

// unknowns_test.go - missing data never turns into a quiet zero: an unknown withholding or dividend
// growth is projected as stated (gross, 0%) and the rules that depend on it say so.

import (
	"strings"
	"testing"
)

func TestAnUnknownWithholdingIsFlagged(t *testing.T) {
	a := sample()
	a.Lines[3].Country = "KR" // no pack rates Korea
	r := CheckPortfolio(a, homeFX)
	if c := ruleOf(t, r, "withholding"); c.Status != NeedsLook || !strings.Contains(c.Detail, "DEUT") {
		t.Errorf("withholding %+v", c)
	}
	if c := ruleOf(t, r, "yoc_year_10"); c.Status == Pass || !strings.Contains(c.Detail, "gross for DEUT") {
		t.Errorf("yoc %+v", c)
	}
}

func TestAFundWithNoGrowthRateIsFlagged(t *testing.T) {
	a := sample()
	a.Lines[7].DividendGrowthPct = nil // FWLD: no source for a fund's dividend growth
	r := CheckPortfolio(a, homeFX)
	if c := ruleOf(t, r, "growth_after_10"); c.Status != NeedsLook || !strings.Contains(c.Detail, "FWLD") {
		t.Errorf("growth_after_10 %+v", c)
	}
	notes := strings.Join(r.Lines[7].Notes, " ")
	if !strings.Contains(notes, "projected at 0%") {
		t.Errorf("line notes %q", notes)
	}
}

func TestYoCYear10WithNoStartMoneyUsesAllMoney(t *testing.T) {
	a := sample()
	a.StartAmount = 0
	r := CheckPortfolio(a, homeFX)
	c := ruleOf(t, r, "yoc_year_10")
	if c.Status == Skipped || !strings.Contains(c.Detail, "on all money paid in (no start amount)") {
		t.Errorf("%+v", c)
	}
}
