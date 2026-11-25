package pbcli

// watchadd_test.go - pb watchlist add.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchlistAdd(t *testing.T) {
	data := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(data, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("watchlist.yaml", "as_of: \"2031-03-08\"\nsource: test\nitems:\n  - {ticker: RLTY, kind: stock, mic: XNYS, currency: USD}\n")
	write("runs/2031-03-08/screen.yaml", "funds:\n  - {rank: 1, ticker: FINC, name: Example Nasdaq Premium, isin: IE0000000104, mic: XLON, currency: USD, bucket: income, reason: x, instrument_id: i-finc}\n")
	write("screen/funds.yaml", "rows:\n  - {ticker: FHDY, name: Example High Dividend, isin: IE0000000103, mic: XLON, currency: GBP}\n  - {ticker: FHDY, name: Example High Dividend, isin: IE0000000103, mic: XAMS, currency: EUR, instrument_id: i-fhdy}\n")
	out := run(t, data, "watchlist", "add", "FINC", "FHDY@XAMS", "RLTY", "--run", "2031-03-08")
	if !strings.Contains(out, "FINC (etf, XLON USD)") || !strings.Contains(out, "FHDY (etf, XAMS EUR)") || !strings.Contains(out, `"RLTY"`) {
		t.Fatalf("out %s", out)
	}
	var w Watchlist
	readYAMLFile(t, filepath.Join(data, "watchlist.yaml"), &w)
	if len(w.Items) != 3 || w.Items[1].InstrumentID != "i-finc" || w.Items[2].InstrumentID != "i-fhdy" || w.Items[2].Kind != kindETF {
		t.Fatalf("items %+v", w.Items)
	}
	var out2 strings.Builder
	root := NewRoot(&out2)
	root.SetArgs([]string{"--data", data, "watchlist", "add", "NOPE"})
	root.SetErr(&out2)
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "NOPE") {
		t.Fatalf("unknown name: %v", err)
	}
}
