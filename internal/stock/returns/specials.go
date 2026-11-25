package returns

// specials.go - relatively large special dividends: the screener and
// Eulerpool's ledger carry no special flag, so a payment is called special when it is at least
// SpecialMultiple times the median of the other payments within two years either side of it, and
// it is not part of a pattern: a final dividend that recurs at the same time each year, or a raise
// that the next (or previous) payment confirms, is regular.

import (
	"sort"
	"time"
)

// SpecialMultiple is how much larger than the regular payment a special one is.
const SpecialMultiple = 2.0

// SpecialYears is how far back the ledger is searched.
const SpecialYears = 5

// Special summarises the special payments of the last SpecialYears years.
type Special struct {
	Count       int      `yaml:"count" json:"count"`
	LastExDate  string   `yaml:"last_ex_date" json:"last_ex_date"`
	MaxMultiple float64  `yaml:"max_multiple" json:"max_multiple"`
	SharePct    float64  `yaml:"share_of_dividends_pct" json:"share_of_dividends_pct"`
	ExDates     []string `yaml:"ex_dates" json:"ex_dates"`
	Rule        string   `yaml:"rule" json:"rule"`
}

// Specials finds the special payments; nil when there are none (or too few payments to judge).
func Specials(ps []Payment, now time.Time) *Special {
	ps = sortedPayments(ps)
	cut := now.AddDate(-SpecialYears, 0, 0).Format("2006-01-02")
	var total float64
	out := &Special{Rule: "a payment at least 2x the median of the other payments within two years of it, last 5 years"}
	var specialSum float64
	for i, p := range ps {
		if p.ExDate < cut || p.Amount <= 0 {
			continue
		}
		total += p.Amount
		t, err := time.Parse("2006-01-02", p.ExDate)
		if err != nil {
			continue
		}
		lo, hi := t.AddDate(-2, 0, 0).Format("2006-01-02"), t.AddDate(2, 0, 0).Format("2006-01-02")
		var near []float64
		for j, q := range ps {
			if j != i && q.ExDate >= lo && q.ExDate <= hi && q.Amount > 0 {
				near = append(near, q.Amount)
			}
		}
		if len(near) < 3 {
			continue
		}
		sort.Float64s(near)
		med := near[len(near)/2]
		if len(near)%2 == 0 {
			med = (near[len(near)/2-1] + near[len(near)/2]) / 2
		}
		if m := p.Amount / med; m >= SpecialMultiple && !recurs(ps, i, t) {
			out.Count++
			out.ExDates = append(out.ExDates, p.ExDate)
			out.LastExDate = p.ExDate
			specialSum += p.Amount
			if m > out.MaxMultiple {
				out.MaxMultiple = *round(m, 2)
			}
		}
	}
	if out.Count == 0 || total <= 0 {
		return nil
	}
	out.SharePct = *round(specialSum/total*100, 1)
	return out
}

// similar is two payments within a factor of 1/0.6 of each other.
func similar(a, b float64) bool { return a > 0 && b > 0 && a/b >= 0.6 && b/a >= 0.6 }

// recurs reports a payment that matches its neighbour in the ledger (a raise, or a new level) or a
// payment about a year before or after it (a yearly final dividend).
func recurs(ps []Payment, i int, t time.Time) bool {
	p := ps[i]
	if (i > 0 && similar(ps[i-1].Amount, p.Amount)) || (i+1 < len(ps) && similar(ps[i+1].Amount, p.Amount)) {
		return true
	}
	for _, yrs := range []int{-1, 1} {
		c := t.AddDate(yrs, 0, 0)
		lo, hi := c.AddDate(0, 0, -60).Format("2006-01-02"), c.AddDate(0, 0, 60).Format("2006-01-02")
		for j, q := range ps {
			if j != i && q.ExDate >= lo && q.ExDate <= hi && similar(q.Amount, p.Amount) {
				return true
			}
		}
	}
	return false
}
