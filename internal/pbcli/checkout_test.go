package pbcli

// checkout_test.go - every pb portfolio check writes its own file.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEachCheckWritesItsOwnFile(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	alloc := "lines:\n  - {ticker: A, kind: stock, weight_pct: 60, yield_pct: 5, country: ZZ, dividend_currency: XTS}\n  - {ticker: F, kind: etf_acc, weight_pct: 40, trading_currency: USD}\n"
	for _, n := range []string{"allocation_income.yaml", "fit_probe.yaml"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte(alloc), 0o644)
		run(t, data, "portfolio", "check", filepath.Join(dir, n))
	}
	for _, n := range []string{"check_income.yaml", "check_fit_probe.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Error(err)
		}
	}
	if checkFileFor("runs/2031-03-08/allocation_final.yaml") != filepath.Join("runs", "2031-03-08", "check_final.yaml") {
		t.Error(checkFileFor("runs/2031-03-08/allocation_final.yaml"))
	}
}
