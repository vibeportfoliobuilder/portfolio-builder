package rules

import (
	"strconv"
	"testing"
)

// The ISINs are made up: only their two-letter domicile matters.
func TestFundWithholdingByDomicile(t *testing.T) {
	for isin, want := range map[string]string{
		"US0000000001": "US B 25", // a US fund is a US payer (Testland's US rate)
		"IE0000000002": "IE S 0",  // the country pack's fund_withholding: a UCITS fund pays gross
		"LU0000000003": "LU S 0",
		"DE0000000004": "DE B 20", // no fund rate: the domicile's dividend rate
	} {
		w := FundWithholding(isin)
		if w == nil {
			t.Errorf("%s: no rate", isin)
			continue
		}
		if got := w.Country + " " + w.Tier + " " + ftoa(w.RatePct); got != want {
			t.Errorf("%s = %s, want %s", isin, got, want)
		}
	}
	if w := FundWithholding("KR0000000005"); w != nil {
		t.Errorf("a fund from a domicile no pack rates got %+v", w)
	}
	// The check charges a US fund line its 25%, an Irish one nothing, and says when it knows no rate.
	us, ok1 := lineWithholding(Line{Ticker: "FUSA", Kind: KindETFDist, ISIN: "US0000000001"})
	ie, ok2 := lineWithholding(Line{Ticker: "FIRL", Kind: KindETFDist, ISIN: "IE0000000002"})
	_, ok3 := lineWithholding(Line{Ticker: "FKOR", Kind: KindETFDist, ISIN: "KR0000000005"})
	if us != 25 || ie != 0 || !ok1 || !ok2 || ok3 {
		t.Errorf("US fund %v %v, IE fund %v %v, KR fund known %v", us, ok1, ie, ok2, ok3)
	}
}

func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
