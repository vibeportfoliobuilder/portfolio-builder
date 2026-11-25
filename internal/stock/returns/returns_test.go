package returns

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// monthly builds closes from "2015-01" on, one per month, from a function of the month number.
func monthly(n int, f func(i int) float64) []Close {
	var out []Close
	t := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		out = append(out, Close{Month: t.AddDate(0, i, 0).Format("2006-01"), Close: f(i)})
	}
	return out
}

func near(p *float64, want, tol float64) bool { return p != nil && math.Abs(*p-want) <= tol }

func TestPriceCAGRIsCompoundedFromTheMonthYearsBack(t *testing.T) {
	// 121 months doubling every 60: 10 years of ~14.87% a year.
	cs := monthly(121, func(i int) float64 { return 100 * math.Pow(2, float64(i)/60) })
	cur := cs[len(cs)-1].Close
	if v, _ := PriceCAGR(cs, cur, 5); !near(v, 14.87, 0.01) {
		t.Errorf("5y = %v", v)
	}
	if v, _ := PriceCAGR(cs, cur, 10); !near(v, 14.87, 0.01) {
		t.Errorf("10y = %v", v)
	}
	if v, why := PriceCAGR(cs[60:], cur, 10); v != nil || why != NoHistory {
		t.Errorf("a 5-year series gave a 10y CAGR %v (%q)", v, why)
	}
}

func TestTotalReturnReinvestsAtTheExMonthClose(t *testing.T) {
	// A flat price of 100 and 1.00 paid every quarter: price CAGR 0, total return ~4.06%/y.
	cs := monthly(61, func(int) float64 { return 100 })
	var ps []Payment
	for i := 3; i < 61; i += 3 {
		ps = append(ps, Payment{ExDate: cs[i].Month + "-15", Amount: 1})
	}
	pc, _ := PriceCAGR(cs, 100, 5)
	tr, _ := TotalReturnCAGR(cs, ps, 100, 5)
	if !near(pc, 0, 1e-9) {
		t.Errorf("price CAGR = %v", pc)
	}
	// 20 reinvested payments of 1%: 1.01^20 over 5 years.
	if want := (math.Pow(math.Pow(1.01, 20), 0.2) - 1) * 100; !near(tr, want, 0.01) {
		t.Errorf("total return = %v, want %.2f", tr, want)
	}
	// A dividend in the start month is not bought: the share was bought after it went ex.
	ps0 := append([]Payment{{ExDate: cs[0].Month + "-02", Amount: 50}}, ps...)
	if tr2, _ := TotalReturnCAGR(cs, ps0, 100, 5); *tr2 != *tr {
		t.Errorf("a start-month dividend counted: %v vs %v", *tr2, *tr)
	}
}

func TestComputeSaysWhyAFigureIsMissing(t *testing.T) {
	cs := monthly(30, func(i int) float64 { return 10 + float64(i) })
	f := Compute(Series{Closes: cs, Payments: []Payment{{ExDate: "2016-06-01", Amount: 0.2}}, Distributions: true,
		DividendGrowth: map[string]*float64{"1y": ptr(4)}})
	if f.PriceCAGR["1y"] == nil || f.TotalReturn["1y"] == nil || f.DividendGrowth["1y"] == nil {
		t.Fatalf("1y missing: %+v", f)
	}
	if f.NA["price_cagr_pct.5y"] != NoHistory || f.NA["total_return_pct.10y"] != NoHistory || f.NA["dividend_growth_pct.5y"] != NoHistory {
		t.Errorf("reasons = %v", f.NA)
	}
	// A distributing fund with no distribution source: price yes, total return and growth "no source".
	e := Compute(Series{Closes: cs})
	if e.PriceCAGR["1y"] == nil || e.NA["total_return_pct.1y"] != NoSource || e.NA["dividend_growth_pct.1y"] != NoSource {
		t.Errorf("dist fund: %+v", e)
	}
	// An accumulating fund: total return is its price return; dividend growth "accumulating".
	a := Compute(Series{Closes: cs, Accumulating: true})
	if a.TotalReturn["1y"] == nil || *a.TotalReturn["1y"] != *a.PriceCAGR["1y"] || a.NA["dividend_growth_pct.1y"] != Accumulating {
		t.Errorf("acc fund: %+v", a)
	}
	// A stock that pays nothing: total return is the price return, growth "no dividend".
	g := Compute(Series{Closes: cs, Distributions: true})
	if g.TotalReturn["1y"] == nil || *g.TotalReturn["1y"] != *g.PriceCAGR["1y"] || g.NA["dividend_growth_pct.1y"] != NoDividend {
		t.Errorf("non-payer: %+v", g)
	}
}

func TestBetaOfALeveredSeriesIsItsLever(t *testing.T) {
	// The benchmark wiggles; the name moves 1.5x its monthly return.
	bench := monthly(61, func(i int) float64 { return 100 + 10*math.Sin(float64(i)/3) })
	name := []Close{{Month: bench[0].Month, Close: 50}}
	for i := 1; i < len(bench); i++ {
		r := bench[i].Close/bench[i-1].Close - 1
		name = append(name, Close{Month: bench[i].Month, Close: name[i-1].Close * (1 + 1.5*r)})
	}
	if b := Beta(name, bench, 60); !near(b, 1.5, 0.01) {
		t.Errorf("beta = %v", b)
	}
	if b := Beta(name[:10], bench, 60); b != nil {
		t.Errorf("beta over 9 months = %v", *b)
	}
}

func TestImpliedBetaMeasuresTheFallInEachCovered(t *testing.T) {
	// 2019-01..2023-12: the benchmark falls 20% in 2020 and 25% in 2022; the name 10% and 25%.
	mk := func(d20, d22 float64) []Close {
		var out []Close
		for y := 2019; y <= 2023; y++ {
			for m := 1; m <= 12; m++ {
				k := fmt.Sprintf("%d-%02d", y, m)
				v := 100.0
				if k == "2020-03" {
					v = 100 * (1 - d20)
				}
				if k == "2022-09" {
					v = 100 * (1 - d22)
				}
				out = append(out, Close{Month: k, Close: v})
			}
		}
		return out
	}
	ib, used := ImpliedBeta(mk(0.10, 0.25), mk(0.20, 0.25), Corrections)
	if !near(ib, 0.75, 0.001) || len(used) != 2 {
		t.Errorf("implied beta = %v over %v", ib, used)
	}
}

func TestTTMYieldEndsAtTheLastExDate(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	ps := []Payment{{"2025-08-01", 1}, {"2025-11-01", 1}, {"2026-02-01", 1}, {"2026-05-01", 1}, {"2026-08-01", 1}}
	if y := TTMYield(ps, 100, now); !near(y, 4, 1e-9) {
		t.Errorf("yield = %v", y)
	}
	if y := TTMYield([]Payment{{"2024-01-01", 1}}, 100, now); !near(y, 0, 0) {
		t.Errorf("a lapsed payer yields %v", y)
	}
}

func ptr(v float64) *float64 { return &v }

func TestAnOutsizedPaymentIsLeftOutOfTheTotalReturn(t *testing.T) {
	cs := monthly(13, func(int) float64 { return 2 })
	f := Compute(Series{Closes: cs, Distributions: true, Payments: []Payment{{ExDate: cs[6].Month + "-10", Amount: 1.8}}})
	if !near(f.TotalReturn["1y"], 0, 1e-9) || len(f.Notes) != 1 {
		t.Errorf("total return %v notes %v", f.TotalReturn["1y"], f.Notes)
	}
}

func TestTTMYieldCountsAnAnnualPayerOnce(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	// Ex-dates 2024-06-01, 2025-05-29, 2026-04-02: the last two sit 308 days apart.
	ps := []Payment{{"2023-06-01", 1.5}, {"2024-06-01", 1.6}, {"2025-05-29", 1.8}, {"2026-04-02", 1.9}}
	if y := TTMYield(ps, 40, now); !near(y, 4.75, 1e-9) {
		t.Errorf("yield = %v, want one payment of 1.9 over 40", y)
	}
}

func TestMaxDrawdownIsTheWorstPeakToTrough(t *testing.T) {
	cs := []Close{{"2020-01", 100}, {"2020-02", 120}, {"2020-03", 90}, {"2020-04", 130}, {"2020-05", 78}, {"2020-06", 140}}
	d, from, to := MaxDrawdown(cs)
	if d == nil || *d != -40 || from != "2020-04" || to != "2020-05" {
		t.Errorf("%v %s %s", d, from, to)
	}
	if d, _, _ := MaxDrawdown(cs[:1]); d != nil {
		t.Errorf("one close: %v", *d)
	}
}
