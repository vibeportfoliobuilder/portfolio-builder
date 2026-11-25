package pbcli

// derive_ledger.go - a dividend ledger that missed a share split. the broker's payout ratio times EPS is the second source for k.

import (
	"fmt"
	"math"
	"reflect"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// splitFactors are the k-for-1 splits the check recognises.
var splitFactors = []float64{2, 3, 4, 5, 10}

const (
	// A ledger more than this many times the broker's implied dividend is a unit question, not growth.
	unsplitMinRatio = 1.8
	// How close the ratio must sit to a split factor.
	unsplitTolerance = 0.2
)

// unsplitLedger divides the payments after the break by k when the ledger's trailing dividend per
// share is about k times the broker's payout × EPS and the ledger jumps by about k at one payment.
// Without a broker payout, a jump that the cash dividends do not show is only noted.
func unsplitLedger(d *eulerpool.Dossier, lyPayoutPct float64) string {
	h := d.Dividends
	if h == nil || h.TTMPerShare == nil || *h.TTMPerShare <= 0 || len(h.Payments) < 2 {
		return ""
	}
	from, jump := "", 0.0
	for i := len(h.Payments) - 1; i > 0; i-- {
		if a, b := h.Payments[i-1].DPS, h.Payments[i].DPS; a > 0 && b/a >= unsplitMinRatio {
			from, jump = h.Payments[i].ExDate, b/a
			break
		}
	}
	if from == "" {
		return ""
	}
	eps := ttmEPS(d)
	if lyPayoutPct <= 0 || eps <= 0 {
		if sh := d.Shares.Outstanding; sh != nil && len(d.Years) > 0 {
			if paid := d.Years[len(d.Years)-1].Statements.DividendsPaid; paid != nil && *paid > 0 && *h.TTMPerShare**sh > 2.5*math.Abs(*paid) {
				return fmt.Sprintf("pb derive: the ledger jumps %.1f× at %s and its trailing dividend × shares is %.1f× FY%d's cash dividends: an unadjusted split or a special? needs a look",
					jump, from, *h.TTMPerShare**sh/math.Abs(*paid), d.Years[len(d.Years)-1].FiscalYear)
			}
		}
		return ""
	}
	implied := lyPayoutPct / 100 * eps
	r := *h.TTMPerShare / implied
	if r < unsplitMinRatio {
		return ""
	}
	for _, k := range splitFactors {
		if math.Abs(r/k-1) <= unsplitTolerance && math.Abs(jump/k-1) <= 0.6 {
			note := eulerpool.UnsplitLedger(d, k, from)
			return note + fmt.Sprintf(" (broker payout %g%% × EPS %g = %.4g per share against the ledger's %g)", lyPayoutPct, eps, implied, *h.TTMPerShare*k)
		}
	}
	return fmt.Sprintf("pb derive: the ledger's trailing dividend %g is %.2f× the broker's payout × EPS (%.4g) and jumps %.1f× at %s; no split factor fits: needs a look",
		*h.TTMPerShare, r, implied, jump, from)
}

// ttmEPS is the TTM diluted EPS, else net income over the shares outstanding.
func ttmEPS(d *eulerpool.Dossier) float64 {
	if d.TTM == nil {
		return 0
	}
	if e := d.TTM.Statements.EPSDiluted; e != nil && *e > 0 {
		return *e
	}
	if ni, sh := d.TTM.Statements.NetIncome, d.Shares.Outstanding; ni != nil && sh != nil && *sh > 0 {
		return *ni / *sh
	}
	return 0
}

// sameStatements compares two statement blocks by value (their fields are pointers).
func sameStatements(a, b eulerpool.Statements) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := range va.NumField() {
		x, y := va.Field(i), vb.Field(i)
		if x.IsNil() != y.IsNil() || (!x.IsNil() && x.Elem().Float() != y.Elem().Float()) {
			return false
		}
	}
	return true
}
