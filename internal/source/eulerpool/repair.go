package eulerpool

// repair.go - corrections for vendor artifacts (pershare.go has the per-share cross-checks); the
// first three the scenario analyst fixed by hand in the first live run. Repair works on a stored dossier in memory (pb derive), never on the file:
//   - a half-year reporter's TTM block sums four half-years, so every flow is doubled (TEP, ADM, GFI);
//   - the dividend ledger repeats one payment a day apart in another unit (0.066 and 0.09 for one UK name);
//   - a company that moved to interim payments has its latest year halved by the vendor's
//     frequency normalisation (BNP 5.80 read as 2.90, a "-60.7%" cut).

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// Repair applies every correction and returns one note per change; ratios are recomputed when the
// TTM or the ledger moved.
func Repair(d *Dossier) []string {
	var notes []string
	notes = append(notes, repairHalfYearTTM(d)...)
	if d.Dividends != nil {
		notes = append(notes, dedupePayments(d)...)
		notes = append(notes, repairFrequencySwitch(d)...)
	}
	notes = append(notes, repairDividendCash(d)...)
	if len(notes) > 0 {
		finishRatios(d)
	}
	// After finishRatios, which would write the net-income figures back.
	notes = append(notes, perShareEarnings(d)...)
	notes = append(notes, checkDebtDrop(d)...)
	return notes
}

// distinctPeriods drops repeated quarters (the same revenue and net income under a period end a few
// days apart), oldest first.
func distinctPeriods(qs []Period) []Period {
	var out []Period
	for _, q := range qs {
		if n := len(out); n > 0 && daysBetween(out[n-1].PeriodEnd, q.PeriodEnd) <= 15 && sameNum(out[n-1].Statements.Revenue, q.Statements.Revenue) &&
			sameNum(out[n-1].Statements.NetIncome, q.Statements.NetIncome) {
			out[n-1] = q
			continue
		}
		out = append(out, q)
	}
	return out
}

func sameNum(a, b *float64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func daysBetween(a, b string) int {
	ta, ok1 := parseDay(a)
	tb, ok2 := parseDay(b)
	if !ok1 || !ok2 {
		return math.MaxInt32
	}
	return int(math.Abs(tb.Sub(ta).Hours() / 24))
}

// repairHalfYearTTM rebuilds the TTM flows from the last two half-years when the reported periods are
// six months apart and the TTM revenue is about twice the last fiscal year's.
func repairHalfYearTTM(d *Dossier) []string {
	if d.TTM == nil || d.TTM.Statements.Revenue == nil || len(d.Years) == 0 {
		return nil
	}
	fy := d.Years[len(d.Years)-1].Statements.Revenue
	qs := distinctPeriods(d.Quarters)
	if fy == nil || *fy <= 0 || len(qs) < 2 {
		return nil
	}
	a, b := qs[len(qs)-2], qs[len(qs)-1]
	gap := daysBetween(a.PeriodEnd, b.PeriodEnd)
	r := *d.TTM.Statements.Revenue / *fy
	if gap < 150 || gap > 200 || r < 1.7 || r > 2.3 {
		return nil
	}
	t := &d.TTM.Statements
	sum := func(dst **float64, x, y *float64) {
		switch {
		case x != nil && y != nil:
			v := *x + *y
			*dst = &v
		case *dst != nil:
			v := **dst / 2
			*dst = &v
		}
	}
	as, bs := a.Statements, b.Statements
	before := *t.Revenue
	sum(&t.Revenue, as.Revenue, bs.Revenue)
	sum(&t.GrossProfit, as.GrossProfit, bs.GrossProfit)
	sum(&t.OpIncome, as.OpIncome, bs.OpIncome)
	sum(&t.NetIncome, as.NetIncome, bs.NetIncome)
	sum(&t.EPSDiluted, as.EPSDiluted, bs.EPSDiluted)
	sum(&t.DandA, as.DandA, bs.DandA)
	sum(&t.OCF, as.OCF, bs.OCF)
	sum(&t.Capex, as.Capex, bs.Capex)
	sum(&t.FCF, as.FCF, bs.FCF)
	sum(&t.DividendsPaid, as.DividendsPaid, bs.DividendsPaid)
	return []string{fmt.Sprintf("pb repair: TTM summed four half-years (revenue %s against FY %s); rebuilt from the half-years ending %s and %s, and the payout, P/E and Debt/FCF recomputed",
		num(before), num(*fy), a.PeriodEnd, b.PeriodEnd)}
}

func num(v float64) string { return fmt.Sprintf("%.6g", v) }

// fyOf is the fiscal year an ex-date falls in, from the fiscal year ends in the statements
// (calendar years when none is known).
func fyOf(d *Dossier, ex string) int {
	t, ok := parseDay(ex)
	if !ok {
		return 0
	}
	month, dayN := 12, 31
	if n := len(d.Years); n > 0 {
		if e, ok := parseDay(d.Years[n-1].PeriodEnd); ok {
			month, dayN = int(e.Month()), e.Day()
		}
	}
	end := time.Date(t.Year(), time.Month(month), dayN, 0, 0, 0, 0, time.UTC)
	if t.After(end) {
		return t.Year() + 1
	}
	return t.Year()
}

// dedupePayments drops a payment made within a week of the one before it (the same dividend
// repeated, often in another unit), from the ledger, the fiscal-year totals and the TTM, then
// recomputes the streaks and CAGRs.
func dedupePayments(d *Dossier) []string {
	h := d.Dividends
	var keep []Payment
	var dropped []Payment
	for _, p := range h.Payments {
		if n := len(keep); n > 0 && daysBetween(keep[n-1].ExDate, p.ExDate) <= 7 {
			dropped = append(dropped, p)
			continue
		}
		keep = append(keep, p)
	}
	if len(dropped) == 0 {
		return nil
	}
	h.Payments = keep
	var notes []string
	lastEx, _ := parseDay(h.LastExDate)
	from := day(lastEx.AddDate(0, 0, -ttmWindowDays))
	for _, p := range dropped {
		if p.ExDate > from && h.TTMPerShare != nil {
			h.TTMPerShare = round(f(*h.TTMPerShare-p.DPS), 6)
		}
		fy := fyOf(d, p.ExDate)
		for i := range h.ByFY {
			y := &h.ByFY[i]
			if y.FiscalYear != fy || y.Count <= 1 {
				continue
			}
			y.Total = *round(f(y.Total-p.DPS), 6)
			y.Count--
			y.Normalised = nil
			if h.PaymentsPerYear != nil && y.FullyPaid && y.Count != int(*h.PaymentsPerYear) {
				y.Normalised = round(f(y.Total/float64(y.Count)**h.PaymentsPerYear), 6)
			}
		}
		notes = append(notes, fmt.Sprintf("pb repair: the dividend ledger repeats a payment (ex %s, %g, within a week of the one before); dropped from FY%d, the TTM and the growth rates",
			p.ExDate, p.DPS, fy))
	}
	if h.TTMPerShare != nil && d.Price.Current != nil && *d.Price.Current > 0 {
		h.Yield = round(f(*h.TTMPerShare / *d.Price.Current * 100), 2)
	}
	restreak(h)
	return notes
}

func restreak(h *DividendHistory) {
	h.PaymentYears, h.NoCutYears, h.IncreaseStreakYears = 0, 0, 0
	h.CAGR1y, h.CAGR3y, h.CAGR5y, h.CAGR10y = nil, nil, nil, nil
	streaks(h)
}

// repairFrequencySwitch undoes the vendor's normalisation of the latest fiscal year when the trailing
// year's payments show the company now pays that often (an interim plus a final), and reads the
// one-year growth on declared years: an H2 payment is that year's interim, an H1 payment the previous
// year's final (BNP: 2.59 + 2.57 for 2025 against 4.79 for 2024, +7.7%).
func repairFrequencySwitch(d *Dossier) []string {
	h := d.Dividends
	if len(h.ByFY) == 0 || h.PaymentsPerYear == nil {
		return nil
	}
	lastFY := &h.ByFY[len(h.ByFY)-1]
	if lastFY.Normalised == nil || lastFY.Count <= int(*h.PaymentsPerYear) {
		return nil
	}
	lastEx, ok := parseDay(h.LastExDate)
	if !ok {
		return nil
	}
	from := day(lastEx.AddDate(0, 0, -ttmWindowDays))
	var recent []Payment
	for _, p := range h.Payments {
		if p.ExDate > from {
			recent = append(recent, p)
		}
	}
	if len(recent) != lastFY.Count {
		return nil
	}
	for i := 1; i < len(recent); i++ {
		if daysBetween(recent[i-1].ExDate, recent[i].ExDate) < 60 {
			return nil
		}
	}
	old := *lastFY.Normalised
	lastFY.Normalised = nil
	restreak(h)
	note := fmt.Sprintf("pb repair: the company now pays %d times a year; FY%d was normalised to %g by the old frequency, read as %g", lastFY.Count, lastFY.FiscalYear, old, lastFY.Total)
	if g, a, b, y := declaredGrowth(h, lastEx); g != nil {
		h.CAGR1y = g
		note += fmt.Sprintf("; 1y dividend growth on declared years: %g for %d against %g for %d", b, y, a, y-1)
	}
	return []string{note}
}

// declaredGrowth sums payments by declared year (July of Y to June of Y+1 belongs to Y) and compares
// the last complete declared year with the one before; both must lie inside the payment ledger.
func declaredGrowth(h *DividendHistory, lastEx time.Time) (*float64, float64, float64, int) {
	if len(h.Payments) == 0 {
		return nil, 0, 0, 0
	}
	first, _ := parseDay(h.Payments[0].ExDate)
	tot := map[int]float64{}
	for _, p := range h.Payments {
		t, ok := parseDay(p.ExDate)
		if !ok {
			continue
		}
		y := t.Year()
		if t.Month() <= time.June {
			y--
		}
		tot[y] += p.DPS
	}
	var ys []int
	for y := range tot {
		ys = append(ys, y)
	}
	slices.Sort(ys)
	for i := len(ys) - 1; i >= 1; i-- {
		y := ys[i]
		end := time.Date(y+1, time.June, 30, 0, 0, 0, 0, time.UTC)
		start := time.Date(y-1, time.July, 1, 0, 0, 0, 0, time.UTC)
		if !lastEx.After(end) || first.After(start) || ys[i-1] != y-1 || tot[y-1] <= 0 {
			continue
		}
		return round(f((tot[y]/tot[y-1]-1)*100), 2), tot[y-1], tot[y], y
	}
	return nil, 0, 0, 0
}
