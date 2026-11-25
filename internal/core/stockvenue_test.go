package core

import "testing"

// TestAMICIsAskedAsTheBrokersVenue: a screener names a venue by its MIC and a broker by its own
// code, so a resolve asked on the MIC found no listing.
func TestAMICIsAskedAsTheBrokersVenue(t *testing.T) {
	for in, want := range map[string]string{
		"XBUD": "BUX", "xpar": "SBF", " XAMS ": "AEB", "XNAS": "NASDAQ", "XDUB": "ISED",
		"NYSE": "NYSE", "ibis": "IBIS", "FWB2": "FWB2", "": "",
	} {
		if got := StockVenue(in); got != want {
			t.Errorf("StockVenue(%q) = %q, want %q", in, got, want)
		}
	}
}

// A bare symbol means the US listing, so these two decide who may be asked by one.
func TestForeignVenueIsAStatedNonUSMarket(t *testing.T) {
	for _, v := range []string{"NYSE", "nasdaq", "XNAS", "PINK", "AMEX", "", "  "} {
		if ForeignVenue(v) {
			t.Errorf("%q read as foreign", v)
		}
	}
	for _, v := range []string{"SBF", "IBIS", "IBIS2", "FWB2", "BVME", "XPAR", "TSE"} {
		if !ForeignVenue(v) {
			t.Errorf("%q read as US", v)
		}
	}
}
