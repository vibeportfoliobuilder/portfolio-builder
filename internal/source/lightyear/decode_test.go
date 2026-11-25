package lightyear

// decode_test.go - the explore-page decoders on a made-up page: the payload is wrapped the way
// the page ships it (a self.__next_f.push chunk), so a change in the page's shape that breaks the
// decoder shows here first. Every row, id and figure is invented.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// fakePage wraps a payload document in one flight chunk inside some page markup.
func fakePage(t *testing.T, payload string) []byte {
	t.Helper()
	lit, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`<html><script>self.__next_f.push([1,` + string(lit) + `])</script></html>`)
}

const instruments = `"instruments":[` +
	`{"id":"i-aaa","symbol":"AAA","name":"Alpha Yield","exchange":"NYSE","isin":"US0000000001","mic":"XNYS","currency":"USD","issueType":"COMMON_STOCK","summary":{"sector":"Utilities"}},` +
	`{"id":"i-bbb","symbol":"BBB","name":"Beta Grower","exchange":"NASDAQ","isin":"US0000000002","mic":"XNAS","currency":"USD","issueType":"COMMON_STOCK","summary":{"sector":"Technology"}},` +
	`{"id":"i-fff","symbol":"FFF","name":"Example World Income","exchange":"LSE","isin":"IE0000000003","mic":"XLON","currency":"USD","issueType":"ETF","summary":{"distributionPolicy":"Dist"}}]`

func TestDecodeStockPage(t *testing.T) {
	payload := `{"screener":{"page":1,"limit":20,"total":5943,"data":[` +
		`{"instrumentId":"i-aaa","country":"United States","sector":"","price":40.5,"marketCapInUsd":50000,"dividendYield":7.1,"payoutRatio":60,"threeYearDividendGrowth":8,"peRatio":null},` +
		`{"instrumentId":"i-bbb","country":"United States","sector":"Software","dividendYield":null},` +
		`{"instrumentId":"i-gone","country":"Nowhere"}]},` + instruments + `}`
	p, err := DecodeStockPage(fakePage(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 5943 || len(p.Stocks) != 2 {
		t.Fatalf("total %d stocks %d (a row with no instrument is dropped)", p.Total, len(p.Stocks))
	}
	a, b := p.Stocks[0], p.Stocks[1]
	if a.Ticker != "AAA" || a.ISIN != "US0000000001" || a.MIC != "XNYS" || a.Sector != "Utilities" || a.Yield == nil || *a.Yield != 7.1 || a.PE != nil {
		t.Errorf("AAA %+v (an empty row sector takes the summary's; a null figure stays nil)", a)
	}
	if b.Sector != "Software" || b.Yield != nil {
		t.Errorf("BBB %+v (the row's sector wins; a null yield is not zero)", b)
	}
}

func TestDecodeFundPage(t *testing.T) {
	payload := `{"x":{"page":1,"limit":20,"total":1,"data":[{"instrumentId":"i-fff","assetType":"Equity ETFs","fundManager":"Example AM"}]},` + instruments + `}`
	p, err := DecodePage(fakePage(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Funds) != 1 || p.Funds[0].Class != "Dist" || p.Funds[0].AssetType != "Equity ETFs" || p.Funds[0].Manager != "Example AM" {
		t.Errorf("%+v", p.Funds)
	}
}

func TestAChangedPageShapeIsAnError(t *testing.T) {
	for name, body := range map[string][]byte{
		"no payload":  []byte(`<html>nothing here</html>`),
		"no envelope": fakePage(t, `{"rows":[]}`),
		"no join":     fakePage(t, `{"s":{"page":1,"limit":20,"total":1,"data":[{"instrumentId":"i-x"}]},"instruments":[]}`),
	} {
		if _, err := DecodeStockPage(body); err == nil {
			t.Errorf("%s: decoded", name)
		}
	}
}

// TestLiveExplorePage reads the real first page of each screener (make test-live sets PB_LIVE_LIGHTYEAR).
func TestLiveExplorePage(t *testing.T) {
	if os.Getenv("PB_LIVE_LIGHTYEAR") == "" {
		t.Skip("set PB_LIVE_LIGHTYEAR=1 to read the public screener")
	}
	c := NewClient()
	sp, err := c.StockPageAt(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Stocks) == 0 || sp.Total < 1000 || sp.Stocks[0].ISIN == "" {
		t.Errorf("stocks page: %d rows, total %d", len(sp.Stocks), sp.Total)
	}
	fp, err := c.Page(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.Funds) == 0 || fp.Funds[0].ISIN == "" {
		t.Errorf("funds page: %+v", fp)
	}
}
