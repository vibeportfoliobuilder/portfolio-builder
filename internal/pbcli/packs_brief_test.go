package pbcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// pb packs --brief keeps what the rules and costs need and drops sources and routing tables.
func TestPacksBrief(t *testing.T) {
	out := run(t, t.TempDir(), "packs", "--brief")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, k := range []string{"active", "withholding", "transaction_tax", "fees", "strategy", "strategy_unset"} {
		if _, ok := m[k]; !ok {
			t.Errorf("no %s: %s", k, out)
		}
	}
	for _, k := range []string{"sources", "market_by_country", "built_in_country", "built_in_broker", "macro_areas"} {
		if strings.Contains(out, `"`+k+`"`) {
			t.Errorf("%s is printed: %s", k, out)
		}
	}
	if full := run(t, t.TempDir(), "packs"); len(out) >= len(full) {
		t.Errorf("brief %d bytes, full %d", len(out), len(full))
	}
}

type flatFee struct{}

func (flatFee) Currency(string) (string, bool) { return "XTS", true }
func (flatFee) Quote(string, float64) pack.Quote {
	return pack.Quote{Offered: true, Parts: []pack.Part{{Label: "commission", Amount: 1}}, Total: 1}
}

// A broker priced by a fee model shows a quote for its default markets instead of a market table.
func TestPacksBriefFeeModel(t *testing.T) {
	a := pack.Active()
	a.Broker.Markets = nil
	a.Broker.DefaultMarket, a.Broker.DefaultETFMarket = "home", "funds"
	a.Broker = a.Broker.WithFees(flatFee{})
	fees := packsBrief(a)["fees"].(map[string]any)
	q := fees["fee_model_quote_1000"].(map[string]string)
	if q["home"] != "1 XTS (commission 1)" || q["funds"] == "" {
		t.Errorf("%v", fees)
	}
}
