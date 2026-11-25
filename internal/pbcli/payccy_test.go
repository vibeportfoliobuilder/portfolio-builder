package pbcli

import "testing"

// A fund's payout currency is its share class's.
func TestShareClassCurrencyFromTheName(t *testing.T) {
	for name, want := range map[string]string{
		"Fidelity US Quality Income UCITS ETF INC-USD":                       "USD",
		"Example All-World High Dividend Yield UCITS ETF (USD) Distributing": "USD",
		"VanEck Semiconductor UCITS ETF Accum A USD":                         "USD",
		"Example World Equity High Income Active UCITS ETF":                  "",
		"Xtrackers MSCI World UCITS ETF 1C EUR Dist":                         "",
	} {
		if got := shareClassCurrency(name); got != want {
			t.Errorf("%q -> %q, want %q", name, got, want)
		}
	}
}
