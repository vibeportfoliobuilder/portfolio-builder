package core

// currency.go - the one currency question with no document in it: GBX is a
// hundredth of a pound, so two codes can name the same money.

import "strings"

// NormaliseListingCurrency turns the exchange's quoting unit into a currency.
// GBX values are scaled where they are read, so what a record stores is GBP.
func NormaliseListingCurrency(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if c == "GBX" {
		return "GBP"
	}
	return c
}

// CcyEquiv reports whether two currency codes name the same currency, GBX and
// GBP included: the two folded by NormaliseListingCurrency are equal.
func CcyEquiv(a, b string) bool {
	return NormaliseListingCurrency(a) == NormaliseListingCurrency(b)
}

// The minor units.

// minorUnitExact holds the spellings that must be matched with case intact: "GBp" upper-
// cases to the pound, "ILs" to the shekel and "ZAr" to the rand, leaving values 100x too big.
var minorUnitExact = map[string]struct {
	iso    string
	factor float64
}{
	"GBp": {"GBP", 0.01},
	"ILs": {"ILS", 0.01},
	"ZAr": {"ZAR", 0.01},
}

// minorUnitUpper maps the spellings whose upper-cased form is unambiguous.
var minorUnitUpper = map[string]struct {
	iso    string
	factor float64
}{
	"GBX":   {"GBP", 0.01},
	"PENCE": {"GBP", 0.01},
	"ZAC":   {"ZAR", 0.01},
	"ILA":   {"ILS", 0.01},
}

// NormaliseCurrency maps a publisher's code to the ISO code and the factor its
// values need: GBX/GBp/PENCE to GBP at 0.01, ZAc to ZAR, ILA/ILs to ILS.
func NormaliseCurrency(code string) (iso string, factor float64) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", 1
	}
	if m, ok := minorUnitExact[code]; ok {
		return m.iso, m.factor
	}
	up := strings.ToUpper(code)
	if m, ok := minorUnitUpper[up]; ok {
		return m.iso, m.factor
	}
	return up, 1
}

// CurrencyOf reads a publisher's currency text as an ISO code and the factor its values need,
// ("", 1) where it names none: NormaliseCurrency first, so "PENCE" is GBP at 0.01, then IsCurrencyCode.
func CurrencyOf(text string) (iso string, factor float64) {
	iso, factor = NormaliseCurrency(text)
	if !IsCurrencyCode(iso) {
		return "", 1
	}
	return iso, factor
}

// currencySymbols are the printed symbols a publisher may put on an amount, by the
// code each stands for. A symbol not here names no currency rather than a guess.
var currencySymbols = map[string]string{"$": "USD", "€": "EUR", "£": "GBP", "¥": "JPY", "₣": "CHF"}

// CurrencyOfSymbol is the ISO code a printed currency symbol stands for, "" for any
// other text: "€" is EUR, "¥" JPY, and an unknown "₹" is no currency at all.
func CurrencyOfSymbol(sym string) string { return currencySymbols[strings.TrimSpace(sym)] }

// IsCurrencyCode reports whether a cell is an ISO 4217 alphabetic code, which is
// the only thing a currency column may contribute.
func IsCurrencyCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := range len(s) {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}
