package pbcli

// lookthrough_test.go - the investor's names inside a fund.

import (
	"slices"
	"testing"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

func TestLikedNamesComeFromTheStrategy(t *testing.T) {
	e := &env{data: t.TempDir(), now: time.Now}
	if got := e.likedNames(); !slices.Equal(got, []string{"AAPL"}) {
		t.Errorf("%v (the test strategy's liked_tickers)", got)
	}
}

// Made-up holdings, weights and ISINs.
func TestFavouritesMatchByISINSymbolOrName(t *testing.T) {
	hs := []eulerpool.FundHolding{{Name: "Alpha Chips", Pct: 5.1}, {Name: "Beta Fruit", Pct: 4.2}, {Name: "Gamma Soft", Symbol: "GSFT", Pct: 3.3},
		{Name: "Delta Foundry Manufacturing Co", Pct: 1.7}, {Name: "Epsilon Bank PLC", ISIN: "GB0000000061", Pct: 0.8}, {Name: "Zeta Networks Inc", Pct: 0.4}}
	favs := []favName{{"ACHP", "Alpha Chips Corp", ""}, {"GSFT", "", ""}, {"DFND", "Delta Foundry Manufacturing", "US0000000062"},
		{"EBNK", "Epsilon", "GB0000000061"}, {"ZNET", "Zeta Networks Inc", ""}, {"ORLT", "Omega Realty Corp", ""}}
	got := favouritesIn(hs, favs)
	if len(got) != 4 || got[0] != (Favourite{"ACHP", 5.1}) || got[1].Ticker != "GSFT" || got[2].Ticker != "DFND" || got[3].Ticker != "EBNK" {
		t.Errorf("%+v (ZNET at 0.4%% is under the line)", got)
	}
	if favouritesIn(nil, favs) != nil || favouritesIn(hs[:2], favs[5:]) == nil {
		t.Error("no holdings must be nil, holdings without a match an empty list")
	}
}
