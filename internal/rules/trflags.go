package rules

// trflags.go - the total-return flags. A flag means "keep only with a stated reason to hold",
// never an exclusion; missing data is "needs_look". The limits are the investor's strategy
// (high_yield_flag_pct, high_yield_min_price_cagr_pct, min_yield_plus_growth_pct); an unset one
// skips its flag.

import (
	"fmt"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// Flagged is a total-return flag's status when the name breaches it.
const Flagged = "flag"

// TotalReturnFlags runs the high-yield price-trend flag (R1b) and the yield-plus-growth flag (R3)
// on a stock's yield, net yield, 10-year price CAGR and 5-year dividend CAGR (percent; nil = unknown).
func TotalReturnFlags(yield, netYield, priceCAGR10y, growth5y *float64) []Check {
	st := pack.Active().Strategy
	r1b := Check{Name: "R1b"}
	switch {
	case st.HighYieldFlagPct == nil || st.HighYieldMinPriceCAGRPct == nil:
		r1b.Status, r1b.Note = Skipped, "the high-yield price-trend flag (high_yield_flag_pct, high_yield_min_price_cagr_pct) is "+pack.UnsetNote
	case yield == nil:
		r1b.Limit = fmt.Sprintf("yield >= %g%% needs a 10y price CAGR >= %g%%", *st.HighYieldFlagPct, *st.HighYieldMinPriceCAGRPct)
		r1b.Status, r1b.Note = NeedsLook, "yield unknown"
	case *yield < *st.HighYieldFlagPct:
		r1b.Limit = fmt.Sprintf("yield >= %g%% needs a 10y price CAGR >= %g%%", *st.HighYieldFlagPct, *st.HighYieldMinPriceCAGRPct)
		r1b.Status, r1b.Note = Skipped, fmt.Sprintf("yield %.2f%% is under %g%%", *yield, *st.HighYieldFlagPct)
	case priceCAGR10y == nil:
		r1b.Limit = fmt.Sprintf("yield >= %g%% needs a 10y price CAGR >= %g%%", *st.HighYieldFlagPct, *st.HighYieldMinPriceCAGRPct)
		r1b.Status, r1b.Note = NeedsLook, "no 10-year price CAGR"
	default:
		r1b.Limit = fmt.Sprintf("yield >= %g%% needs a 10y price CAGR >= %g%%", *st.HighYieldFlagPct, *st.HighYieldMinPriceCAGRPct)
		v := *priceCAGR10y
		r1b.Value = &v
		r1b.Status = Pass
		if v < *st.HighYieldMinPriceCAGRPct {
			r1b.Status, r1b.Note = Flagged, "high yield with a falling price: needs a reason to hold"
		}
	}
	r3 := Check{Name: "R3"}
	if st.MinYieldPlusGrowthPct != nil {
		r3.Limit = fmt.Sprintf("net yield + 5y dividend CAGR >= %g%%", *st.MinYieldPlusGrowthPct)
	}
	switch {
	case st.MinYieldPlusGrowthPct == nil:
		r3.Status, r3.Note = Skipped, "the yield-plus-growth flag (min_yield_plus_growth_pct) is "+pack.UnsetNote
	case yield != nil && *yield == 0:
		r3.Status, r3.Note = Skipped, "no dividend"
	case yield == nil || netYield == nil:
		r3.Status, r3.Note = NeedsLook, "net yield unknown"
	case growth5y == nil:
		r3.Status, r3.Note = NeedsLook, "no 5-year dividend CAGR"
	default:
		v := round(*netYield+*growth5y, 2)
		r3.Value = &v
		r3.Status = Pass
		if v < *st.MinYieldPlusGrowthPct {
			r3.Status, r3.Note = Flagged, "low yield plus growth: needs a reason to hold"
		}
	}
	return []Check{r1b, r3}
}
