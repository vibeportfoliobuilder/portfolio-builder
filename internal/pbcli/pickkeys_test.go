package pbcli

// pickkeys_test.go - picks name screen rows by instrument id, not ticker.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/returns"
)

func TestFromPicksMatchesByInstrumentID(t *testing.T) {
	data := t.TempDir()
	rd := filepath.Join(data, "runs", "2031-03-08")
	_ = os.MkdirAll(rd, 0o755)
	screen := `as_of: x
stocks:
  - {rank: 1, ticker: DUP, name: Other DUP, isin: US0000000001, mic: XNYS, currency: USD, group: Yielder, instrument_id: i-us, reason: x}
  - {rank: 2, ticker: DUP, name: Dup Home Co, isin: ZZ0000093257, mic: XHOM, currency: XTS, group: Yielder, instrument_id: i-hu, reason: x}
funds:
  - {rank: 1, ticker: FHDY, name: Example All-World High Div, isin: IE0000000103, exchange: LSE, mic: XLON, currency: GBP, bucket: income, reason: x}
  - {rank: 2, ticker: FHDY, name: Example All-World High Div, isin: IE0000000103, exchange: AEX, mic: XAMS, currency: EUR, bucket: income, reason: x}
`
	_ = os.WriteFile(filepath.Join(rd, "screen.yaml"), []byte(screen), 0o644)
	p := filepath.Join(t.TempDir(), "picks.json")
	_ = os.WriteFile(p, []byte(`{"run":"2031-03-08","stocks":["DUP"],"funds":["FHDY"],
"stock_keys":[{"ticker":"DUP","instrument_id":"i-hu","isin":"ZZ0000093257","mic":"XHOM"}],
"fund_keys":[{"ticker":"FHDY","isin":"IE0000000103","mic":"XAMS"}]}`), 0o600)
	run(t, data, "watchlist", "from-picks", p)
	var w Watchlist
	readYAMLFile(t, filepath.Join(data, "watchlist.yaml"), &w)
	if len(w.Items) != 2 || w.Items[0].InstrumentID != "i-hu" || w.Items[0].Currency != "XTS" || w.Items[1].MIC != "XAMS" || w.Items[1].Currency != "EUR" {
		t.Fatalf("%+v", w.Items)
	}
}

func TestStableGroupsComeFromTheLedgerStreaks(t *testing.T) {
	c := ScreenCandidate{Ticker: "PEP", Group: "none", CapBand: "large", Returns: &returns.Figures{Streaks: map[string]int{"no_cut": 25, "increase": 12}}}
	addStableGroups(&c)
	if c.Group != "Stable Grower" || len(c.Groups) != 2 || c.Groups[1] != "Stable Payer" {
		t.Errorf("%+v", c)
	}
	m := ScreenCandidate{Ticker: "X", Group: "Yielder", Groups: []string{"Yielder"}, CapBand: "mid", Returns: c.Returns}
	addStableGroups(&m)
	if len(m.Groups) != 1 {
		t.Errorf("a mid cap got a Stable group: %+v", m.Groups)
	}
}
