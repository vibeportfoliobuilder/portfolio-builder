package screen

// withholding_test.go - the stated table and the tiers it bands into. The rates
// are an operator's own, so what is tested is the BANDING and the country key.

import (
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// TestTheTiersBandTheStatedRates: the boundaries are the whole of this function,
// and 15 falling into A rather than B is a tier a reader would act on wrongly.
func TestTheTiersBandTheStatedRates(t *testing.T) {
	for _, tc := range []struct {
		rate float64
		want string
	}{
		{0, TierS},
		{10, TierA},
		{12.8, TierA},
		{14.99, TierA},
		{15, TierB},
		{26.4, TierB},
		{29.99, TierB},
		{30, TierC},
		{35, TierC},
	} {
		if got := TierOf(tc.rate); got != tc.want {
			t.Errorf("TierOf(%v) = %q, want %q", tc.rate, got, tc.want)
		}
	}
}

// TestEveryStatedRateHasATier: the table and the bands are declared apart, so a
// rate nothing bands would print an empty tier beside a real number.
func TestEveryStatedRateHasATier(t *testing.T) {
	for code, rate := range pack.Active().Country.Withholding {
		if TierOf(rate) == "" {
			t.Errorf("%s withholds %v and falls in no tier", code, rate)
		}
	}
}

// TestTheRateComesFromTheISSUERAndNotTheVenue: a Canadian company on a New York
// line is the case this whole column exists for: Canada's rate at source, not the US rate.
func TestTheRateComesFromTheISSUERAndNotTheVenue(t *testing.T) {
	ry := Row{Ticker: "CABK", ISIN: "CA0000000071", Exchange: "NYSE", Country: "US"}
	tax := TaxOf(ry, false)
	if tax == nil {
		t.Fatal("a row with a Canadian ISIN resolved to no country at all")
	}
	// 22, not the country table's 20: the test broker's override for Canada.
	if tax.Country != "CA" || tax.Rate != 22 || tax.Tier != TierB {
		t.Errorf("CABK = %+v, want CA 22 %s", *tax, TierB)
	}
}

// TestTheTwoOriginsSpellACountryDifferently: secdef states an alpha-2 and the
// screener origin a name, and the same company must read the same rate either way.
func TestTheTwoOriginsSpellACountryDifferently(t *testing.T) {
	for _, r := range []Row{
		{Country: "US"},
		{Country: "us"},
		{Country: "United States"},
		{ISIN: "US0000000072"},
	} {
		tax := TaxOf(r, false)
		if tax == nil || tax.Country != "US" || tax.Rate != 25 || tax.Tier != TierB {
			t.Errorf("%+v resolved to %v, want US 25 %s", r, tax, TierB)
		}
	}
}

// TestACountryTheTableDoesNotNameHasNoTier: a missing rate is absent and never a
// zero, which would read as the one tier that means tax-free.
func TestACountryTheTableDoesNotNameHasNoTier(t *testing.T) {
	for _, r := range []Row{
		{Country: "JP"},
		{Country: "Japan"},
		{Country: ""},
		{ISIN: "XS0000000000"},
		{ISIN: "X"},
	} {
		if tax := TaxOf(r, false); tax != nil {
			t.Errorf("%+v resolved to %+v, want no tier at all", r, *tax)
		}
	}
}

// TestAnISINOutsideTheTableFallsBackToTheStatedCountry: the prefix is preferred
// because it names the issuer, but a prefix nothing rates must not end the search.
func TestAnISINOutsideTheTableFallsBackToTheStatedCountry(t *testing.T) {
	tax := TaxOf(Row{ISIN: "XS1234567890", Country: "DE"}, false)
	if tax == nil || tax.Country != "DE" || tax.Rate != 20 {
		t.Errorf("an XS line in Germany resolved to %v, want DE 20 (Testland's table)", tax)
	}
}

// TestTheStatedRateIsUsedAsItIs: the pack states the rate withheld at source (a reclaim-only treaty
// cap is not a rate the investor gets), and the tier is read off that rate.
func TestTheStatedRateIsUsedAsItIs(t *testing.T) {
	for _, tc := range []struct {
		country string
		rate    float64
		tier    string
	}{
		{"NO", 22, TierB},
		{"DK", 24, TierB},
		{"SE", 25, TierB},
		{"FR", 12, TierA},
		{"CH", 30, TierC},
	} {
		tax := TaxOf(Row{Country: tc.country}, false)
		if tax == nil || tax.Rate != tc.rate || tax.Tier != tc.tier {
			t.Errorf("%s = %v, want %v in tier %s", tc.country, tax, tc.rate, tc.tier)
		}
	}
}

// TestAnOffshorePayerWithholdsNothing: a Cayman or Bermuda holdco pays its
// dividend gross, whatever exchange it trades on and wherever it operates.
func TestAnOffshorePayerWithholdsNothing(t *testing.T) {
	for _, code := range []string{"BM", "KY", "JE", "HK", "SG"} {
		tax := TaxOf(Row{Country: code}, false)
		if tax == nil || tax.Rate != 0 || tax.Tier != TierS {
			t.Errorf("%s = %v, want 0 in tier %s", code, tax, TierS)
		}
	}
	// The case this matters for: an operating company in one country, incorporated
	// in another, listed in a third. The PAYER is what withholds.
	if tax := TaxOf(Row{Ticker: "BGM", ISIN: "KYG1234567890", Country: "China", Exchange: "XNAS"}, false); tax == nil || tax.Country != "KY" {
		t.Errorf("a Cayman holdco of a Chinese business resolved to %v, want KY", tax)
	}
}

// TestTheOriginsCountryNamesResolveToTheirCodes: the screener origin states a
// name and not a code, so a country in the table that it spells out must still land.
func TestTheOriginsCountryNamesResolveToTheirCodes(t *testing.T) {
	for name, code := range map[string]string{
		"Israel": "IL", "Luxembourg": "LU", "Bermuda": "BM", "Cayman Islands": "KY",
		"Virgin Islands (British)": "VG", "Hong Kong": "HK", "Singapore": "SG",
		"Isle of Man": "IM", "The Bahamas": "BS", "Monaco": "MC", "Guernsey": "GG",
	} {
		if got := TaxCountry(Row{Country: name}); got != code {
			t.Errorf("%q resolved to %q, want %q", name, got, code)
		}
	}
}

// TestTheUnratedStayUnrated: each of these is a country the sets list and the
// table deliberately does not name, and a dash there is the honest answer.
func TestTheUnratedStayUnrated(t *testing.T) {
	for _, code := range pack.Active().Country.Unrated {
		if tax := TaxOf(Row{Country: code}, false); tax != nil {
			t.Errorf("%s was given %+v; it is on the unrated list for a reason", code, *tax)
		}
	}
}

// A receipt's ISIN is its venue's, so a US prefix says nothing about the payer: a Taiwanese
// issuer's receipt is sourced in Taiwan, a UK one's in the UK (ISINs made up).
func TestAReceiptIsRatedAtItsPayersCountry(t *testing.T) {
	for _, c := range []struct {
		row  Row
		code string
		rate float64
		tier string
	}{
		{Row{Ticker: "TWRC", ISIN: "US0000000081", Type: "ADR", Payer: "TW"}, "TW", 21, TierB},
		{Row{Ticker: "GBRC", ISIN: "US0000000082", Type: "ADR", Payer: "GB"}, "GB", 0, TierS},
		{Row{Ticker: "HKRC", ISIN: "US0000000083", Type: "ADR", Payer: "HK"}, "HK", 0, TierS},
	} {
		got := TaxOf(c.row, false)
		if got == nil || got.Country != c.code || got.Rate != c.rate || got.Tier != c.tier {
			t.Errorf("%s = %+v, want %s %v %s", c.row.Ticker, got, c.code, c.rate, c.tier)
		}
	}
}

// A receipt nobody names a payer for reads no rate: US 30 would be a stated falsehood.
func TestAReceiptWithNoPayerIsUnrated(t *testing.T) {
	if tax := TaxOf(Row{Ticker: "X", ISIN: "US0000000000", Type: "ADR", Country: "US"}, false); tax != nil {
		t.Errorf("a receipt with no payer read %+v", *tax)
	}
	if got := TaxOf(Row{Ticker: "X", ISIN: "US0000000000", Type: "ADR", Country: "JP"}, false); got != nil {
		t.Errorf("JP has no rate in the table, read %+v", *got)
	}
	if got := TaxCountry(Row{Ticker: "X", ISIN: "US0000000000", Type: "ADR", Country: "Taiwan"}); got != "TW" {
		t.Errorf("a receipt's stated country read %q", got)
	}
}

func TestADomesticCompanyIsUntouched(t *testing.T) {
	for _, r := range []Row{
		{Ticker: "USCO", ISIN: "US0000000091", Type: "COMMON", Country: "US"},
		{Ticker: "USCO", ISIN: "US0000000091"},
		{Ticker: "USRT", ISIN: "US0000000092", Type: "REIT", Country: "US"},
	} {
		if got := TaxOf(r, false); got == nil || got.Country != "US" || got.Rate != 25 {
			t.Errorf("%+v = %+v", r, got)
		}
	}
}

func TestStatePayersNamesOnlyTheRowsTheUniverseStates(t *testing.T) {
	rows := []Row{{Ticker: "TWRC"}, {Ticker: "USCO"}}
	StatePayers(rows, map[string]string{"TWRC": "TW"})
	if rows[0].Payer != "TW" || rows[1].Payer != "" {
		t.Errorf("rows = %+v", rows)
	}
}
