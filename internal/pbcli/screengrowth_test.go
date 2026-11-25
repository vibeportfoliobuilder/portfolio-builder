package pbcli

// screengrowth_test.go - pb screen draft ranks the Growth group by size, not by its token yield
// (profile 2031-03-08: balanced tilt, no starting-yield target).

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScreenDraftRanksGrowthBySize(t *testing.T) {
	data := t.TempDir()
	_ = os.MkdirAll(filepath.Join(data, "screen"), 0o755)
	rows := `as_of: "2031-03-08"
source: test fixture (hand-written)
rows:
- {ticker: SMALL, name: Small Growth, isin: US0000000011, mic: XNAS, currency: USD, country: United States, sector: Technology, market_cap_usd_m: 3000, yield_pct: 1.9, div_growth_3y_pct: 20}
- {ticker: MEGA, name: Mega Growth, isin: US0000000012, mic: XNAS, currency: USD, country: United States, sector: Technology, market_cap_usd_m: 3000000, yield_pct: 0.2}
- {ticker: NODIV, name: No Dividend, isin: US0000000013, mic: XNAS, currency: USD, country: United States, sector: Technology, market_cap_usd_m: 400000}
`
	if err := os.WriteFile(filepath.Join(data, "screen", "stocks.yaml"), []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, data, "screen", "draft", "--run", "2031-03-08")
	var doc ScreenDoc
	readYAMLFile(t, filepath.Join(data, "runs", "2031-03-08", "screen.yaml"), &doc)
	var got []string
	for _, s := range doc.Stocks {
		got = append(got, s.Ticker)
	}
	if len(got) != 3 || got[0] != "MEGA" || got[1] != "NODIV" || got[2] != "SMALL" {
		t.Errorf("growth order %v, want MEGA, NODIV, SMALL", got)
	}
}
