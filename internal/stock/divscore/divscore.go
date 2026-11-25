// Package divscore is the dividend safety score, ported from the old web server
// (internal/stock/derive/dividendscore.go and internal/server/dividendscore.go on branch old).
//
// The ABSOLUTE half (Score) is six pillars worth 100 points; a criterion the data states nothing
// for shrinks the denominator rather than scoring zero, two hard caps bind after the pillars are
// added, and a band names the result. pb derive stores it in derived.yaml. The RELATIVE half
// (Rank) places four raw factors among the names being read and combines 60/40 with the safety;
// it depends on the set, so pb report build strikes it and nothing stores it. Pure: no I/O.
package divscore

import (
	"fmt"
	"math"
)

// The six pillars and what each is worth out of a hundred.
const (
	PillarPayout    = "payout coverage"     // 30
	PillarBalance   = "balance sheet"       // 25
	PillarCash      = "cash generation"     // 15
	PillarRecord    = "dividend record"     // 15
	PillarQuality   = "business quality"    // 10
	PillarRecession = "recession behaviour" // 5
)

// What payout coverage was scored against.
const (
	BasisEarningsAndCash = "EARNINGS_AND_CASH"
	BasisFundsFromOps    = "FUNDS_FROM_OPERATIONS"
)

// Which cash flow the first index factor was struck on, and which return the second.
const (
	CashFlowThreeYearAverage = "THREE_YEAR_AVERAGE_FREE_CASH_FLOW"
	CashFlowFundsFromOps     = "FUNDS_FROM_OPERATIONS"
	ReturnOnEquity           = "RETURN_ON_EQUITY"
	ReturnOnInvestedCapital  = "RETURN_ON_INVESTED_CAPITAL"
)

// The caps: a cut in the last ten years holds the score at 40; a payout above both its
// numerators (earnings and FCF, or FFO and AFFO for a REIT) at 35.
const (
	CapCut         = "a cut in the last ten years"
	CapCutCeiling  = 40.0
	CapPayout      = "a payout above everything it is paid out of"
	CapPayoutLimit = 35.0
	CapCutYears    = 10
)

// The scoreable points a score needs; a REIT's payout pillar has one source fewer here.
const (
	MinScoreable     = 55.0
	MinScoreableREIT = 44.0
)

// Why there is no score: nothing to score, and too little read to score it.
const (
	NoteNoDividend  = "no dividend score: the company pays no dividend"
	GapThinCoverage = "no dividend score: only %.0f of the hundred points could be scored, below the %.0f it needs; the pillars name what the data does not state"
)

// Input is what one company is scored on, all optional: nil is "not stated". Percent fields are
// percent, multiples are multiples.
type Input struct {
	REIT         bool
	PaysDividend bool

	PayoutEarnings, PayoutFCF *float64 // operating company
	PayoutFFO, PayoutAFFO     *float64 // REIT
	PayoutBasisNote           string   // e.g. "operating cash flow, no FFO filed"

	NetDebtToEBITDA *float64
	InterestCover   *float64
	DebtToEquity    *float64

	FCFCover  *float64 // free cash flow over the dividend, times
	FCFMargin *float64 // free cash flow over revenue
	FFOCover  *float64 // REIT
	FFOMargin *float64 // REIT

	IncreaseYears *float64
	PaymentYears  *float64

	ROIC      *float64
	OpMargin  *float64
	Beta      *float64
	ImpliedB  *float64
	CutYears  []int // fiscal years the dividend fell, partial years excluded
	ScoreYear int   // the year the cut window ends in

	// The four index factors, raw.
	CashFlowToDebt  *float64
	CashFlowBasis   string
	ROE             *float64
	ReturnBasis     string
	Yield           *float64
	DividendGrowth5 *float64
}

// Pillar is one of the six with what it was scored over.
type Pillar struct {
	Name      string   `yaml:"name" json:"name"`
	Earned    float64  `yaml:"earned" json:"earned"`
	Available float64  `yaml:"available" json:"available"`
	Measured  []string `yaml:"measured,omitempty" json:"measured,omitempty"`
	Unscored  []string `yaml:"unscored,omitempty" json:"unscored,omitempty"`
}

// Cap is one hard limit, stated whether or not it bound.
type Cap struct {
	Name    string  `yaml:"name" json:"name"`
	Ceiling float64 `yaml:"ceiling" json:"ceiling"`
	Applied bool    `yaml:"applied" json:"applied"`
	Why     string  `yaml:"why,omitempty" json:"why,omitempty"`
}

// Factors are the four index factors as the company states them, unranked.
type Factors struct {
	CashFlowToDebt  *float64 `yaml:"cash_flow_to_debt,omitempty" json:"cash_flow_to_debt,omitempty"`
	ReturnOnEquity  *float64 `yaml:"return_on_equity,omitempty" json:"return_on_equity,omitempty"`
	Yield           *float64 `yaml:"yield,omitempty" json:"yield,omitempty"`
	DividendGrowth5 *float64 `yaml:"dividend_growth_5y,omitempty" json:"dividend_growth_5y,omitempty"`
	ReturnBasis     string   `yaml:"return_basis,omitempty" json:"return_basis,omitempty"`
	CashFlowBasis   string   `yaml:"cash_flow_basis,omitempty" json:"cash_flow_basis,omitempty"`
}

// Score is the absolute half, independent of any other company.
type Score struct {
	Safety        *float64 `yaml:"safety,omitempty" json:"safety,omitempty"`
	Band          string   `yaml:"band,omitempty" json:"band,omitempty"`
	Earned        float64  `yaml:"earned" json:"earned"`
	Scoreable     float64  `yaml:"scoreable" json:"scoreable"`
	Basis         string   `yaml:"basis" json:"basis"`
	Pillars       []Pillar `yaml:"pillars" json:"pillars"`
	Caps          []Cap    `yaml:"caps" json:"caps"`
	Factors       Factors  `yaml:"factors" json:"factors"`
	TenYearRecord bool     `yaml:"ten_year_record" json:"ten_year_record"`
	Note          string   `yaml:"note,omitempty" json:"note,omitempty"`
}

// rung is one band of a ladder: the bound, and what clearing it earns.
type rung struct{ at, points float64 }

// down scores a figure where smaller is better, against a ladder written best first.
func down(rs []rung) func(float64) float64 {
	return func(v float64) float64 {
		for _, r := range rs {
			if v <= r.at {
				return r.points
			}
		}
		return 0
	}
}

// up scores a figure where larger is better.
func up(rs []rung) func(float64) float64 {
	return func(v float64) float64 {
		for _, r := range rs {
			if v >= r.at {
				return r.points
			}
		}
		return 0
	}
}

type pillar struct{ out Pillar }

// add scores one criterion, or names it as unscored.
func (p *pillar) add(name string, worth float64, v *float64, score func(float64) float64) {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		p.out.Unscored = append(p.out.Unscored, name)
		return
	}
	p.out.Available += worth
	p.out.Earned += score(*v)
	p.out.Measured = append(p.out.Measured, name)
}

// Band is a safety figure in the published vocabulary.
func Band(safety float64) string {
	switch {
	case safety > 80:
		return "Very Safe"
	case safety > 60:
		return "Safe"
	case safety > 40:
		return "Borderline"
	case safety > 20:
		return "Unsafe"
	default:
		return "Very Unsafe"
	}
}

// Compute strikes the absolute half.
func Compute(in Input) Score {
	out := Score{Basis: BasisEarningsAndCash}
	if in.REIT {
		out.Basis = BasisFundsFromOps
	}
	out.Pillars = []Pillar{payoutPillar(in), balancePillar(in), cashPillar(in), recordPillar(in), qualityPillar(in), recessionPillar(in)}
	for _, p := range out.Pillars {
		out.Earned += round2(p.Earned)
		out.Scoreable += p.Available
	}
	out.Earned, out.Scoreable = round2(out.Earned), round2(out.Scoreable)
	out.Factors = Factors{CashFlowToDebt: in.CashFlowToDebt, CashFlowBasis: in.CashFlowBasis, ReturnOnEquity: in.ROE,
		ReturnBasis: in.ReturnBasis, Yield: positive(in.Yield), DividendGrowth5: in.DividendGrowth5}
	out.TenYearRecord = in.PaymentYears != nil && *in.PaymentYears >= 10
	out.Caps = caps(in)
	if !in.PaysDividend {
		out.Note = NoteNoDividend
		return out
	}
	floor := MinScoreable
	if in.REIT {
		floor = MinScoreableREIT
	}
	if out.Scoreable < floor {
		out.Note = fmt.Sprintf(GapThinCoverage, out.Scoreable, floor)
		return out
	}
	s, _ := Capped(out.Earned/out.Scoreable*100, out.Caps)
	s = round2(s)
	out.Safety, out.Band = &s, Band(s)
	return out
}

// Capped is v under every cap that applied, and the name of the last one that bound.
func Capped(v float64, cs []Cap) (float64, string) {
	var by string
	for _, c := range cs {
		if c.Applied && v > c.Ceiling {
			v, by = c.Ceiling, c.Name
		}
	}
	return v, by
}

// payoutPillar is 30: a REIT's dividend is paid from funds from operations, on a looser ladder.
func payoutPillar(in Input) Pillar {
	p := pillar{out: Pillar{Name: PillarPayout}}
	if in.REIT {
		l := down([]rung{{70, 15}, {80, 12}, {90, 8}, {100, 4}, {110, 1}})
		p.add("payout against adjusted funds from operations", 15, in.PayoutAFFO, l)
		name := "payout against funds from operations"
		if in.PayoutBasisNote != "" {
			name += " (" + in.PayoutBasisNote + ")"
		}
		p.add(name, 15, in.PayoutFFO, l)
		return p.out
	}
	l := down([]rung{{40, 15}, {55, 12}, {70, 8}, {85, 4}, {100, 1}})
	p.add("payout against earnings", 15, in.PayoutEarnings, l)
	p.add("payout against free cash flow", 15, in.PayoutFCF, l)
	return p.out
}

// balancePillar is 25: what the dividend competes with.
func balancePillar(in Input) Pillar {
	p := pillar{out: Pillar{Name: PillarBalance}}
	lev := down([]rung{{1, 10}, {2, 8}, {3, 6}, {4, 3}, {5, 1}})
	if in.REIT {
		lev = down([]rung{{4, 10}, {5, 8}, {6, 6}, {7, 3}, {8, 1}})
	}
	p.add("net debt against operating earnings", 10, in.NetDebtToEBITDA, lev)
	p.add("interest cover", 8, positive(in.InterestCover), up([]rung{{12, 8}, {8, 6}, {5, 4}, {3, 2}, {1.5, 1}}))
	// Debt to equity on negative equity is not an answer; it would score full marks for insolvency.
	p.add("debt against equity", 7, positive(in.DebtToEquity), down([]rung{{0.5, 7}, {1, 5}, {1.5, 4}, {2.5, 2}, {4, 1}}))
	return p.out
}

// cashPillar is 15: how many times the cash covers the dividend, and the cash margin.
func cashPillar(in Input) Pillar {
	p := pillar{out: Pillar{Name: PillarCash}}
	cover := up([]rung{{3, 8}, {2, 6}, {1.5, 4}, {1.2, 2}, {1, 1}})
	if in.REIT {
		p.add("funds from operations against the dividend", 8, in.FFOCover, cover)
		p.add("funds from operations against revenue", 7, in.FFOMargin, up([]rung{{40, 7}, {30, 5}, {20, 3}, {0.01, 1}}))
		return p.out
	}
	p.add("free cash flow against the dividend", 8, in.FCFCover, cover)
	p.add("free cash flow against revenue", 7, nonzero(in.FCFMargin), up([]rung{{15, 7}, {10, 5}, {5, 3}, {0.01, 1}}))
	return p.out
}

// recordPillar is 15: years of rises and years of payments, which differ.
func recordPillar(in Input) Pillar {
	p := pillar{out: Pillar{Name: PillarRecord}}
	p.add("consecutive years of rises", 8, in.IncreaseYears, up([]rung{{25, 8}, {10, 6}, {5, 4}, {1, 2}}))
	p.add("consecutive years of payments", 7, in.PaymentYears, up([]rung{{25, 7}, {15, 5}, {10, 4}, {5, 2}, {1, 1}}))
	return p.out
}

// qualityPillar is 10: the return the dividend is a claim on.
func qualityPillar(in Input) Pillar {
	p := pillar{out: Pillar{Name: PillarQuality}}
	ret := up([]rung{{15, 5}, {10, 4}, {7, 3}, {4, 2}, {0.01, 1}})
	if in.REIT {
		ret = up([]rung{{8, 5}, {6, 4}, {4, 3}, {2, 2}, {0.01, 1}})
	}
	p.add("return on invested capital", 5, nonzero(in.ROIC), ret)
	p.add("operating margin", 5, nonzero(in.OpMargin), up([]rung{{25, 5}, {15, 4}, {8, 3}, {3, 2}, {0.01, 1}}))
	return p.out
}

// recessionPillar is 5: what the price did when the market fell.
func recessionPillar(in Input) Pillar {
	p := pillar{out: Pillar{Name: PillarRecession}}
	p.add("beta", 3, positive(in.Beta), down([]rung{{0.7, 3}, {0.9, 2}, {1.15, 1}}))
	p.add("what it did in past corrections", 2, in.ImpliedB, down([]rung{{1, 2}, {1.3, 1}}))
	return p.out
}

// caps are the hard limits, stated whether or not they bound.
func caps(in Input) []Cap {
	over := func(p *float64) bool { return p != nil && *p > 100 }
	both := over(in.PayoutEarnings) && over(in.PayoutFCF)
	if in.REIT {
		both = over(in.PayoutFFO) && over(in.PayoutAFFO)
	}
	cut := Cap{Name: CapCut, Ceiling: CapCutCeiling}
	for _, y := range in.CutYears {
		if y >= in.ScoreYear-CapCutYears {
			cut.Applied = true
			cut.Why = fmt.Sprintf("the dividend fell in FY%d", y)
		}
	}
	return []Cap{cut, {Name: CapPayout, Ceiling: CapPayoutLimit, Applied: both}}
}

func positive(p *float64) *float64 {
	if p == nil || *p <= 0 {
		return nil
	}
	return p
}

func nonzero(p *float64) *float64 {
	if p == nil || *p == 0 {
		return nil
	}
	return p
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
