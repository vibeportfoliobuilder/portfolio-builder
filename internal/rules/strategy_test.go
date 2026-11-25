package rules

// strategy_test.go - with no strategy nothing is judged: groups, safe-dividend limits and the
// portfolio rules skip and say why, and no switch is projected.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack/packtest"
)

func withEmptyStrategy(t *testing.T) {
	t.Helper()
	s := packtest.Set()
	s.Strategy = pack.Strategy{}
	pack.Use(s)
	t.Cleanup(packtest.Activate)
}

func TestNoStrategyJudgesNoName(t *testing.T) {
	withEmptyStrategy(t)
	in := Input{Ticker: "AAA", Country: "US", YieldPct: p(7), Growth1y: p(4), Growth3y: p(6), Growth5y: p(9), PayoutPct: p(60), MarketCapUSDm: p(50000)}
	v := Evaluate(in)
	if v.Group != "none" || !strings.Contains(v.GroupBasis, pack.StrategyFile) {
		t.Errorf("group %q basis %q", v.Group, v.GroupBasis)
	}
	if v.Safe != Skipped {
		t.Errorf("safe %q, want skipped", v.Safe)
	}
	// A dividend cut still fails the name: the growth checks need no limit.
	in.Growth5y = p(-20)
	if v := Evaluate(in); v.Safe != Fail {
		t.Errorf("a 20%% cut with no limits set: safe %q", v.Safe)
	}
}

func TestNoStrategySkipsThePortfolioRules(t *testing.T) {
	withEmptyStrategy(t)
	r := CheckPortfolio(sample(), homeFX)
	for _, name := range []string{"etf_share", "weight_cap", "income_cap", "moonshot_income_cap", "fees", "yoc_year_10", "price_growth"} {
		if c := ruleOf(t, r, name); c.Status != Skipped || !strings.Contains(c.Detail, pack.StrategyFile) {
			t.Errorf("%s: %+v", name, c)
		}
	}
	if r.Switch == nil || len(r.Switch.Rows) != 0 || len(r.Switch.Notes) == 0 {
		t.Errorf("switch %+v, want none projected and a note", r.Switch)
	}
}

func TestAMisspelledStrategyKeyIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "profile"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, pack.StrategyFile), []byte("yoc_year_10_pct: 8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pack.LoadStrategy(dir); err == nil || !strings.Contains(err.Error(), "yoc_year_10_pct") {
		t.Errorf("err %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, pack.StrategyFile), []byte("yoc_year10_pct: 8\ncustom: {account_limit: 5}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := pack.LoadStrategy(dir)
	if err != nil || s.YoCYear10Pct == nil || *s.YoCYear10Pct != 8 || s.Custom["account_limit"] == nil {
		t.Errorf("%+v %v", s, err)
	}
}

func TestPackNamesAreCaseInsensitive(t *testing.T) {
	pack.Register("country", "Mixedland", []byte("code: ZZ\nname: Mixedland\ncurrency: XTS\ninflation_pct: 2\n"))
	if !slices.Contains(pack.Names("country"), "mixedland") {
		t.Errorf("names %v", pack.Names("country"))
	}
}
