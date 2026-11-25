package pbcli

// picks_test.go - gate 1 takes 25 to 35 stocks; outside that pb warns and never trims.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFromPicksWarnsOutsideTwentyFiveToThirtyFive(t *testing.T) {
	data := screenData(t)
	run(t, data, "screen", "draft", "--run", "2031-03-07")
	p := filepath.Join(t.TempDir(), "picks.json")
	_ = os.WriteFile(p, []byte(`{"run":"2031-03-07","stocks":["AAA","CCC"],"funds":[]}`), 0o600)
	if out := run(t, data, "watchlist", "from-picks", p); !strings.Contains(out, "gate 1 takes 25 to 35") {
		t.Errorf("no warning for 2 stocks: %s", out)
	}
}
