package eulerpool

// pershare.go - the TTM flows cross-checked against per-share figures. The vendor's trailing window for cash dividends paid can hold five quarters or two annual
// payments (CVX, KO), a half-year row repeats the year's dividend (SU, SAF), or the
// line is missing; its TTM net income can include more than the shareholders' share (ACA, CS).
// The dividend ledger and EPS are per share and agree with the company's own payout, so they win.

import (
	"fmt"
	"math"
	"strings"
)

// The bands inside which the cash and per-share figures count as agreeing.
const (
	perShareLow, perShareHigh = 0.87, 1.15
	// Outside these, the gap is a unit or scrip question rather than a window: noted, nothing changed.
	perShareUnitLow, perShareUnitHigh = 0.33, 3.0
	// Below this the cash line is missing, not wrong.
	perShareMissing = 0.05
	// A latest-quarter total debt below this share of the last fiscal year's is suspect.
	debtDropFactor = 0.5
)

// repairDividendCash replaces the TTM cash dividends paid with the ledger's trailing dividend per
// share times the shares outstanding when the two disagree like a mis-sized window.
func repairDividendCash(d *Dossier) []string {
	h, t, sh := d.Dividends, d.TTM, d.Shares.Outstanding
	if h == nil || t == nil || h.TTMPerShare == nil || *h.TTMPerShare <= 0 || sh == nil || *sh <= 0 || !sameCurrency(d) {
		return nil
	}
	implied := *h.TTMPerShare * *sh
	cash := 0.0
	if t.Statements.DividendsPaid != nil {
		cash = math.Abs(*t.Statements.DividendsPaid)
	}
	r := cash / implied
	switch {
	case r >= perShareLow && r <= perShareHigh:
		return nil
	case r > perShareMissing && (r < perShareUnitLow || r > perShareUnitHigh):
		return []string{fmt.Sprintf("pb repair: TTM cash dividends paid %s is %.2f× the ledger's %g per share × %s m shares = %s; too far apart for a window error (scrip, units or a split?), left as is: needs a look",
			num(cash), r, *h.TTMPerShare, num(*sh), num(implied))}
	}
	why := fmt.Sprintf("%.2f×: the vendor's trailing window, or cash lagging the ex-dates", r)
	if r <= perShareMissing {
		why = "the cash line is missing"
	}
	t.Statements.DividendsPaid = round(&implied, 2)
	return []string{fmt.Sprintf("pb repair: TTM cash dividends paid %s against the ledger's %g per share × %s m shares = %s (%s); the payout, FCF and OCF payouts use the per-share figure",
		num(cash), *h.TTMPerShare, num(*sh), num(implied), why)}
}

// perShareEarnings puts the payout and the P/E on per-share figures when the TTM net income does not
// match EPS × shares (net income including minorities or a double-counted period). It runs after
// finishRatios, which would otherwise write the net-income figures back.
func perShareEarnings(d *Dossier) []string {
	t, sh := d.TTM, d.Shares.Outstanding
	if t == nil || t.Statements.EPSDiluted == nil || *t.Statements.EPSDiluted <= 0 || t.Statements.NetIncome == nil ||
		*t.Statements.NetIncome <= 0 || sh == nil || *sh <= 0 || !sameCurrency(d) {
		return nil
	}
	eps, ni := *t.Statements.EPSDiluted, *t.Statements.NetIncome
	q := ni / (eps * *sh)
	if q >= perShareLow && q <= perShareHigh {
		return nil
	}
	if q < perShareUnitLow || q > perShareUnitHigh {
		// Another unit (an ADR's EPS against the home-currency statements), not a wrong line.
		return nil
	}
	r := &d.Ratios
	var did []string
	if h := d.Dividends; h != nil && h.TTMPerShare != nil && *h.TTMPerShare > 0 {
		r.PayoutRatio = round(f(*h.TTMPerShare/eps*100), 2)
		did = append(did, fmt.Sprintf("payout %g%% = %g / %g", *r.PayoutRatio, *h.TTMPerShare, eps))
	}
	if p := d.Price.Current; p != nil && *p > 0 {
		r.PE = round(f(*p/eps), 2)
		did = append(did, fmt.Sprintf("P/E %g = %g / %g", *r.PE, *p, eps))
	}
	if len(did) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("pb repair: TTM net income %s is %.2f× EPS %g × %s m shares; %s on per-share figures",
		num(ni), q, eps, num(*sh), strings.Join(did, ", "))}
}

// checkDebtDrop repairs a latest-quarter total debt far below the period before it with no matching
// fall in cash: the vendor's newest quarter often lacks its long-term debt, so the sum is short-term
// debt only (SU 5,334 against 18,035; TXN 1,149 against 14,050). The TTM takes the debt of the latest
// earlier period (quarter, else fiscal year) and keeps its own cash.
func checkDebtDrop(d *Dossier) []string {
	if d.TTM == nil || d.TTM.Statements.TotalDebt == nil {
		return nil
	}
	t := &d.TTM.Statements
	var prev *Period
	qs := distinctPeriods(d.Quarters)
	for i := len(qs) - 1; i >= 0 && prev == nil; i-- {
		if qs[i].PeriodEnd < d.TTM.PeriodEnd && qs[i].Statements.TotalDebt != nil && *qs[i].Statements.TotalDebt > 0 {
			prev = &qs[i]
		}
	}
	for i := len(d.Years) - 1; i >= 0 && prev == nil; i-- {
		if d.Years[i].Statements.TotalDebt != nil && *d.Years[i].Statements.TotalDebt > 0 {
			prev = &d.Years[i]
		}
	}
	if prev == nil {
		return nil
	}
	pd := *prev.Statements.TotalDebt
	if *t.TotalDebt >= pd*debtDropFactor {
		return nil
	}
	drop := pd - *t.TotalDebt
	if t.Cash != nil && prev.Statements.Cash != nil && *prev.Statements.Cash-*t.Cash >= drop/2 {
		return nil // the debt was paid down from cash
	}
	old := *t.TotalDebt
	t.TotalDebt = round(&pd, 2)
	t.NetDebt = round(sum(t.TotalDebt, neg(t.Cash)), 2)
	note := fmt.Sprintf("pb repair: the latest quarter's total debt %s is %.2f× the %s balance's %s with no matching fall in cash (the vendor's newest quarter often lacks its long-term debt); the TTM uses %s",
		num(old), old/pd, prev.PeriodEnd, num(pd), num(pd))
	if t.FCF != nil && *t.FCF > 0 {
		d.Ratios.DebtToFCF = round(ratio(t.TotalDebt, t.FCF), 2)
		note += fmt.Sprintf(", Debt/FCF %g", *d.Ratios.DebtToFCF)
	}
	return []string{note}
}

// sameCurrency: per-share and statement figures are compared only in one currency.
func sameCurrency(d *Dossier) bool {
	return d.PriceCurrency == "" || d.Currency == "" || strings.EqualFold(d.PriceCurrency, d.Currency)
}

// UnsplitLedger divides the ledger's payments from ex-date from onwards by k, for a vendor ledger
// that missed a k-for-1 split the prices, EPS and share count already carry (KCR: 2.25 for 0.75
// after a 3:1 split), and recomputes the totals, the trailing yield, the streaks and the CAGRs.
// The caller decides k from a second source; Repair runs after it.
func UnsplitLedger(d *Dossier, k float64, from string) string {
	h := d.Dividends
	if h == nil || k <= 1 {
		return ""
	}
	n := 0
	for i := range h.Payments {
		p := &h.Payments[i]
		if p.ExDate < from {
			continue
		}
		old := p.DPS
		p.DPS = *round(f(old/k), 6)
		fy := fyOf(d, p.ExDate)
		for j := range h.ByFY {
			if y := &h.ByFY[j]; y.FiscalYear == fy {
				y.Total = *round(f(y.Total-old+p.DPS), 6)
				y.Normalised = nil
				if h.PaymentsPerYear != nil && y.FullyPaid && y.Count > 0 && y.Count != int(*h.PaymentsPerYear) {
					y.Normalised = round(f(y.Total/float64(y.Count)**h.PaymentsPerYear), 6)
				}
			}
		}
		n++
	}
	if n == 0 {
		return ""
	}
	if lastEx, ok := parseDay(h.LastExDate); ok {
		win := day(lastEx.AddDate(0, 0, -ttmWindowDays))
		ttm := 0.0
		for _, p := range h.Payments {
			if p.ExDate > win {
				ttm += p.DPS
			}
		}
		h.TTMPerShare = round(&ttm, 6)
		if p := d.Price.Current; p != nil && *p > 0 {
			h.Yield = round(f(ttm / *p * 100), 2)
		}
	}
	restreak(h)
	d.Ratios.DividendYield = h.Yield
	return fmt.Sprintf("pb repair: the dividend ledger's %d payment(s) from %s are not adjusted for a %g-for-1 split that the price, EPS and share count carry; divided by %g", n, from, k, k)
}

// DropSpecials takes the given special payments out of the fiscal-year totals (never a year's only
// payment) and recomputes the streaks and CAGRs, so a one-off does not read as a cut the year after
// (a 2024 special makes the next 1y figure negative). The ledger and the trailing yield keep them.
func DropSpecials(d *Dossier, exDates []string) string {
	h := d.Dividends
	if h == nil || len(exDates) == 0 {
		return ""
	}
	var did []string
	for _, ex := range exDates {
		for _, p := range h.Payments {
			if p.ExDate != ex {
				continue
			}
			fy := fyOf(d, ex)
			for i := range h.ByFY {
				y := &h.ByFY[i]
				if y.FiscalYear != fy || y.Count <= 1 || y.Total-p.DPS <= 0 {
					continue
				}
				y.Total = *round(f(y.Total-p.DPS), 6)
				y.Count--
				y.Normalised = nil
				if h.PaymentsPerYear != nil && y.FullyPaid && y.Count != int(*h.PaymentsPerYear) {
					y.Normalised = round(f(y.Total/float64(y.Count)**h.PaymentsPerYear), 6)
				}
				did = append(did, fmt.Sprintf("%s (%g, FY%d)", ex, p.DPS, fy))
			}
		}
	}
	if len(did) == 0 {
		return ""
	}
	restreak(h)
	return "pb derive: special dividend(s) " + strings.Join(did, ", ") + " left out of the fiscal-year totals, the growth rates and the streaks (kept in the ledger and the trailing yield)"
}

// StreakFloors names the streaks (payment, no_cut, increase) the data cannot close, with the reason:
// the run reaches the ledger's first fully paid year, or stops at a year that is paid but not fully
// paid in the ledger, or at a year with another number of payments than usual (a payment slipping
// across a fiscal-year end: ENB's FY2016–17, BNS's FY2023). Such a streak is "at least N".
func StreakFloors(h *DividendHistory) map[string]string {
	if h == nil {
		return nil
	}
	var full []FYTotal
	paid := map[int]bool{}
	for _, y := range h.ByFY {
		if y.FullyPaid {
			full = append(full, y)
		}
		if y.Total > 0 {
			paid[y.FiscalYear] = true
		}
	}
	n := len(full)
	if n == 0 {
		return nil
	}
	firstPaid := 0
	for _, y := range h.ByFY {
		if y.Total > 0 {
			firstPaid = y.FiscalYear
			break
		}
	}
	// The first paying year with fewer payments is the dividend's start, not a slipped payment.
	odd := func(y FYTotal) bool {
		// (the vendor's normalisation of such a year, total/count×frequency, is no fix: BNS FY2023)
		return h.PaymentsPerYear != nil && y.Count != int(*h.PaymentsPerYear) &&
			!(y.FiscalYear == firstPaid && y.Count < int(*h.PaymentsPerYear))
	}
	// gap says whether the years between two fully paid years were paid (a partial year, not a stop).
	gap := func(a, b FYTotal) string {
		for y := a.FiscalYear + 1; y < b.FiscalYear; y++ {
			if !paid[y] {
				return ""
			}
		}
		return fmt.Sprintf("stops at FY%d, which is paid but not fully paid in the ledger", b.FiscalYear-1)
	}
	// Reaching the first fully paid year is open only when the ledger begins there: earlier rows with
	// no or partial payments are the years before the dividend started.
	start := ""
	if h.ByFY[0].FiscalYear == full[0].FiscalYear {
		start = fmt.Sprintf("reaches the start of the ledger, FY%d", full[0].FiscalYear)
	}
	// step: the run stopped between full[i-1] and full[i]; i == 0 is the ledger start.
	step := func(i int) string {
		switch {
		case i <= 0:
			return start
		case full[i].FiscalYear != full[i-1].FiscalYear+1:
			return gap(full[i-1], full[i])
		case odd(full[i]):
			return fmt.Sprintf("stops at FY%d with %d payments (one across the fiscal-year end?)", full[i].FiscalYear, full[i].Count)
		case odd(full[i-1]):
			return fmt.Sprintf("stops at FY%d with %d payments (one across the fiscal-year end?)", full[i-1].FiscalYear, full[i-1].Count)
		}
		return ""
	}
	out := map[string]string{}
	// The payment run counts years full[n-P..n-1]; it stopped at full[n-P-1].
	if j := n - h.PaymentYears - 1; h.PaymentYears > 0 {
		switch {
		case j < 0:
			if start != "" {
				out["payment"] = start
			}
		case full[j].value() > 0:
			if w := gap(full[j], full[j+1]); w != "" {
				out["payment"] = w
			}
		}
	}
	// The no-cut and increase runs count steps; they stopped between full[i-1] and full[i].
	for name, steps := range map[string]int{"no_cut": h.NoCutYears, "increase": h.IncreaseStreakYears} {
		if i := n - 1 - steps; i >= 0 && (i > 0 || steps > 0) && (i == 0 || full[i-1].value() > 0) {
			if w := step(i); w != "" {
				out[name] = w
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
