package pbcli

// emptystrategy_test.go - a first run, before the questionnaire: an empty strategy file and the
// example packs. pb portfolio check --md must report every rule as not set, never crash.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack/packtest"
)

func TestPortfolioCheckMarkdownWithAnEmptyStrategy(t *testing.T) {
	t.Cleanup(packtest.Activate)
	data := t.TempDir()
	if err := os.MkdirAll(filepath.Join(data, "profile"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "profile", "strategy.yaml"), []byte("# empty until /profile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mix := filepath.Join(data, "mix.yaml")
	if err := os.WriteFile(mix, []byte("start_amount: 10000\nmonthly: 500\nlines:\n  - {ticker: AAA, kind: stock, weight_pct: 60, yield_pct: 4, dividend_growth_pct: 5, country: US, dividend_currency: USD}\n  - {ticker: FFF, kind: etf_acc, weight_pct: 40, trading_currency: EUR, bucket: all_cap}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := run(t, data, "portfolio", "check", mix, "--md")
	for _, want := range []string{"## Portfolio check (EUR)", "etf_share | skipped", "Not projected.", "switch_sell_pct"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}
