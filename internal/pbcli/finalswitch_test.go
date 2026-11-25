package pbcli

// finalswitch_test.go - pb final from-file keeps the page's switch-to-income inputs, else the mix's own.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
)

func TestFinalFromFileKeepsTheSwitchInputs(t *testing.T) {
	data := t.TempDir()
	rd := filepath.Join(data, "runs", "2031-03-08")
	_ = os.MkdirAll(rd, 0o755)
	mix := "currency: XTS\nstart_amount: 100000\nmonthly: 20000\nyears: 10\nstart_year: 2031\nswitch: {bond_cap_pct: 20}\nlines:\n  - {ticker: RLTY, kind: stock, weight_pct: 100, switch: keep}\n"
	_ = os.WriteFile(filepath.Join(rd, "allocation_balanced.yaml"), []byte(mix), 0o644)
	p := filepath.Join(t.TempDir(), "final.json")
	_ = os.WriteFile(p, []byte(`{"run":"2031-03-08","mix":"balanced","start_amount":100000,"monthly":20000,"years":10,"inflation_pct":3.2,"lines":[{"ticker":"RLTY","kind":"stock","weight_pct":100}]}`), 0o600)
	run(t, data, "final", "from-file", p)
	var a rules.Allocation
	readYAMLFile(t, filepath.Join(rd, "allocation_final.yaml"), &a)
	if a.Switch == nil || a.Switch.BondCapPct == nil || *a.Switch.BondCapPct != 20 || a.Lines[0].SwitchRole != "keep" {
		t.Fatalf("the mix's switch block and line override: %+v %+v", a.Switch, a.Lines)
	}
	_ = os.WriteFile(p, []byte(`{"run":"2031-03-08","mix":"balanced","start_amount":100000,"monthly":20000,"years":10,"inflation_pct":3.2,"lines":[{"ticker":"RLTY","kind":"stock","weight_pct":100}],
"switch":{"years":[10,15,20],"sell_pct":60,"bond_cap_pct":30,"bond_yields_pct":[5],"stock_share_pct":40,"glide_years":3,"target_monthly":15000,"higher_monthly":30000}}`), 0o600)
	run(t, data, "final", "from-file", p)
	readYAMLFile(t, filepath.Join(rd, "allocation_final.yaml"), &a)
	if a.Switch == nil || *a.Switch.BondCapPct != 30 || a.Switch.GlideYears != 3 || a.Switch.HigherMonthly != 30000 {
		t.Errorf("the page's switch inputs: %+v", a.Switch)
	}
}
