package pbcli

// divscore.go - the dividend safety score's inputs, read off the stored Eulerpool dossier,
// EDGAR's FFO and the benchmark series (internal/stock/divscore has the formula).

import (
	"math"
	"strconv"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/divscore"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/returns"
)

// scoreInput maps one name's files onto the score's criteria. Nothing is invented: a figure the
// files do not state stays nil and shrinks the denominator.
func scoreInput(d eulerpool.Dossier, out Derived, ed *EdgarDoc, bench []returns.Close, now time.Time) divscore.Input {
	reit := out.Rules.REIT
	in := divscore.Input{REIT: reit, ScoreYear: now.Year(), Yield: out.YieldPct, DividendGrowth5: out.DividendGrowth["5y"]}
	h := d.Dividends
	in.PaysDividend = h != nil && ((h.TTMPerShare != nil && *h.TTMPerShare > 0) || (h.Yield != nil && *h.Yield > 0))
	r := d.Ratios
	in.PayoutEarnings, in.PayoutFCF = lossIsWorst(r.PayoutRatio), lossIsWorst(r.FCFPayoutRatio)
	if reit {
		switch {
		case out.FFOPayoutPct != nil:
			in.PayoutFFO = lossIsWorst(out.FFOPayoutPct)
		case r.OCFPayoutRatio != nil:
			in.PayoutFFO, in.PayoutBasisNote = lossIsWorst(r.OCFPayoutRatio), "operating cash flow, no FFO filed"
		}
		in.FFOCover = timesOf(in.PayoutFFO)
		if ed != nil {
			in.FFOMargin = ffoMargin(d, *ed)
		}
	}
	in.FCFCover = timesOf(r.FCFPayoutRatio)
	in.DebtToEquity = r.DebtToEquity
	in.ROIC, in.OpMargin = r.ROIC, r.OpMargin
	cur := d.TTM
	var last *eulerpool.Period
	if n := len(d.Years); n > 0 {
		last = &d.Years[n-1]
	}
	if cur == nil {
		cur = last
	}
	if cur != nil {
		st := cur.Statements
		var da *float64
		if last != nil {
			da = last.Statements.DandA
		}
		if st.NetDebt != nil && st.OpIncome != nil {
			ebitda := *st.OpIncome
			if da != nil {
				ebitda += math.Abs(*da)
			}
			if ebitda > 0 {
				in.NetDebtToEBITDA = rounded(*st.NetDebt/ebitda, 2)
			}
		}
		if st.FCF != nil && st.Revenue != nil && *st.Revenue > 0 {
			in.FCFMargin = rounded(*st.FCF / *st.Revenue * 100, 2)
		}
	}
	if h != nil {
		inc, pay := float64(h.IncreaseStreakYears), float64(h.PaymentYears)
		in.IncreaseYears, in.PaymentYears = &inc, &pay
		in.CutYears = cutYears(h.ByFY, now.Year())
	}
	if closes := closesOf(d.Price.MonthlyCloses); len(closes) > 0 && len(bench) > 0 {
		in.Beta = returns.Beta(closes, bench, 60)
		in.ImpliedB, _ = returns.ImpliedBeta(closes, bench, returns.Corrections)
	}
	in.CashFlowToDebt, in.CashFlowBasis = cashFlowToDebt(d, out, reit, ed)
	switch {
	case r.ROE != nil && *r.ROE > 0:
		in.ROE, in.ReturnBasis = r.ROE, divscore.ReturnOnEquity
	case r.ROIC != nil && *r.ROIC > 0:
		in.ROE, in.ReturnBasis = r.ROIC, divscore.ReturnOnInvestedCapital
	}
	return in
}

// lossIsWorst reads a negative payout (a loss, or negative free cash flow) as the worst payout
// rather than letting a smaller-is-better ladder give it full marks.
func lossIsWorst(p *float64) *float64 {
	if p == nil || *p >= 0 {
		return p
	}
	v := 999.0
	return &v
}

// timesOf turns a payout in percent into cover: 80% paid out is 1.25 times covered; a negative
// payout is no cover at all.
func timesOf(p *float64) *float64 {
	switch {
	case p == nil || *p == 0:
		return nil
	case *p < 0 || *p >= 999:
		z := 0.0
		return &z
	}
	return rounded(100 / *p, 4)
}

// cutYears names each fiscal year whose (normalised) total fell below the year before, among
// consecutive fully paid years; the current year is partial and never counted.
func cutYears(ys []eulerpool.FYTotal, year int) []int {
	var out []int
	val := func(y eulerpool.FYTotal) float64 {
		if y.Normalised != nil {
			return *y.Normalised
		}
		return y.Total
	}
	for i := 1; i < len(ys); i++ {
		a, b := ys[i-1], ys[i]
		if !a.FullyPaid || !b.FullyPaid || b.FiscalYear != a.FiscalYear+1 || b.FiscalYear >= year || val(a) <= 0 {
			continue
		}
		if val(b) < val(a)*0.995 {
			out = append(out, b.FiscalYear)
		}
	}
	return out
}

// FreeCashFlowYears is how many filed years the first factor averages: one year's working-capital
// swing must not decide it (KO: trailing FCF negative against three good years).
const FreeCashFlowYears = 3

// cashFlowToDebt is the first factor: the three-year average FCF (FFO for a REIT) over total debt.
func cashFlowToDebt(d eulerpool.Dossier, out Derived, reit bool, ed *EdgarDoc) (*float64, string) {
	var debt float64
	if d.TTM != nil && d.TTM.Statements.TotalDebt != nil {
		debt = *d.TTM.Statements.TotalDebt
	}
	if debt <= 0 && len(d.Years) > 0 && d.Years[len(d.Years)-1].Statements.TotalDebt != nil {
		debt = *d.Years[len(d.Years)-1].Statements.TotalDebt
	}
	if debt <= 0 {
		return nil, ""
	}
	if reit && ed != nil && len(ed.FFO) > 0 && ed.FFO[0].FFOm > 0 {
		return rounded(ed.FFO[0].FFOm/debt, 4), divscore.CashFlowFundsFromOps
	}
	var vs []float64
	for i := len(d.Years) - 1; i >= 0 && len(vs) < FreeCashFlowYears; i-- {
		if f := d.Years[i].Statements.FCF; f != nil {
			vs = append(vs, *f)
		}
	}
	if len(vs) == 0 {
		return nil, ""
	}
	var s float64
	for _, v := range vs {
		s += v
	}
	return rounded(s/float64(len(vs))/debt, 4), divscore.CashFlowThreeYearAverage
}

// ffoMargin is EDGAR's newest FFO over the same fiscal year's revenue, in percent.
func ffoMargin(d eulerpool.Dossier, ed EdgarDoc) *float64 {
	for _, f := range ed.FFO {
		if len(f.PeriodEnd) < 4 || f.FFOm <= 0 {
			continue
		}
		fy, _ := strconv.Atoi(f.PeriodEnd[:4])
		for _, y := range d.Years {
			if y.FiscalYear == fy && y.Statements.Revenue != nil && *y.Statements.Revenue > 0 {
				return rounded(f.FFOm / *y.Statements.Revenue * 100, 2)
			}
		}
	}
	return nil
}
