package pbcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A fund's citations go under data/etfs.
func TestAFundsSourcesGoUnderETFs(t *testing.T) {
	data := t.TempDir()
	_ = os.WriteFile(filepath.Join(data, "watchlist.yaml"), []byte("as_of: x\nsource: t\nitems:\n  - {ticker: FCCA, kind: etf}\n  - {ticker: RLTY, kind: stock}\n"), 0o644)
	e := &env{data: data, now: time.Now}
	if got := e.ledgerDir("FCCA"); got != filepath.Join(data, "etfs", "FCCA") {
		t.Errorf("FCCA -> %s", got)
	}
	if got := e.ledgerDir("RLTY"); got != filepath.Join(data, "stocks", "RLTY") {
		t.Errorf("O -> %s", got)
	}
}
