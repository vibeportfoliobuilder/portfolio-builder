package pbcli

// screeninclude_test.go - pb screen draft --include keeps the investor's named
// names whatever the cut, and pb ly screen stocks --append merges pools (mergeStockRows).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScreenDraftIncludesNamedTickers(t *testing.T) {
	data := t.TempDir()
	pool := `rows:
  - {ticker: AAA, name: A, isin: US0000000001, mic: XNYS, currency: USD, country: US, yield_pct: 7, div_growth_3y_pct: 8, market_cap_usd_m: 50000}
  - {ticker: BBB, name: B, isin: US0000000002, mic: XNYS, currency: USD, country: US, yield_pct: 6.5, div_growth_3y_pct: 2, market_cap_usd_m: 30000}
  - {ticker: CCC, name: C, isin: US0000000003, mic: XNYS, currency: USD, country: US, yield_pct: 6.2, div_growth_3y_pct: 1, market_cap_usd_m: 20000}
  - {ticker: ACHP, name: Alpha Chips, isin: US0000000101, mic: XNAS, currency: USD, country: US, yield_pct: 0.03, market_cap_usd_m: 4500000}
`
	if err := os.MkdirAll(filepath.Join(data, "screen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "screen", "stocks.yaml"), []byte(pool), 0o644); err != nil {
		t.Fatal(err)
	}
	out := run(t, data, "screen", "draft", "--run", "2031-03-09", "--stocks", "2", "--include", "achp,ZZZ")
	var res struct {
		Stocks int      `json:"stocks"`
		Notes  []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err, out)
	}
	var doc ScreenDoc
	readYAMLFile(t, filepath.Join(data, "runs", "2031-03-09", "screen.yaml"), &doc)
	if len(doc.Stocks) != 2 || doc.Stocks[0].Ticker != "ACHP" || doc.Stocks[0].Rank != 1 || !strings.Contains(strings.Join(doc.Stocks[0].Flags, ";"), IncludedFlag) {
		t.Fatalf("stocks %+v", doc.Stocks)
	}
	if !strings.Contains(strings.Join(res.Notes, ";"), "include ZZZ: not in data/screen/stocks.yaml") {
		t.Fatalf("notes %v", res.Notes)
	}
}

func TestMergeStockRowsKeepsOneRowPerListing(t *testing.T) {
	a := []ScreenStock{{Ticker: "RLTY", ISIN: "US0000000102", MIC: "XNYS"}, {Ticker: "X", InstrumentID: "i1"}}
	b := []ScreenStock{{Ticker: "RLTY", ISIN: "US0000000102", MIC: "XNYS"}, {Ticker: "X2", InstrumentID: "i1"}, {Ticker: "ACHP", ISIN: "US0000000101", MIC: "XNAS"}}
	m := mergeStockRows(a, b)
	if len(m) != 3 || m[2].Ticker != "ACHP" {
		t.Fatalf("merged %+v", m)
	}
}
