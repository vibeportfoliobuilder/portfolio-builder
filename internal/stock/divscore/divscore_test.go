package divscore

// Ported from branch old: internal/stock/derive/dividendscore_test.go and
// internal/server/dividendscore_test.go. Each case is one where the arithmetic has a plausible
// wrong answer: an absence scoring zero, a cap that does not bite, a REIT read on earnings.

import (
	"strings"
	"testing"
)

func pf(v float64) *float64 { return &v }

// safePayer is good on every pillar; each case breaks exactly one thing.
func safePayer() Input {
	return Input{
		PaysDividend: true, Yield: pf(2.5),
		PayoutEarnings: pf(35), PayoutFCF: pf(38),
		NetDebtToEBITDA: pf(1.4), InterestCover: pf(14), DebtToEquity: pf(0.6),
		FCFCover: pf(2.6), FCFMargin: pf(18),
		IncreaseYears: pf(28), PaymentYears: pf(30),
		ROIC: pf(16), OpMargin: pf(26), Beta: pf(0.6), ImpliedB: pf(0.8),
		ScoreYear: 2026, CashFlowToDebt: pf(0.4), CashFlowBasis: CashFlowThreeYearAverage,
		ROE: pf(30), ReturnBasis: ReturnOnEquity, DividendGrowth5: pf(6),
	}
}

func TestAFullyStatedPayerIsScoredOutOfAHundred(t *testing.T) {
	s := Compute(safePayer())
	if s.Scoreable != 100 {
		t.Errorf("scoreable = %v", s.Scoreable)
	}
	if s.Safety == nil || *s.Safety < 80 || s.Band != "Very Safe" {
		t.Fatalf("safety = %v %q", s.Safety, s.Band)
	}
	if len(s.Pillars) != 6 {
		t.Fatalf("%d pillars", len(s.Pillars))
	}
	var total float64
	for _, p := range s.Pillars {
		if p.Available == 0 || p.Earned > p.Available {
			t.Errorf("%s: %v of %v", p.Name, p.Earned, p.Available)
		}
		total += p.Available
	}
	if total != s.Scoreable || s.Basis != BasisEarningsAndCash {
		t.Errorf("total %v basis %q", total, s.Basis)
	}
}

func TestAnAbsentFigureShrinksTheDenominator(t *testing.T) {
	full := Compute(safePayer())
	in := safePayer()
	in.NetDebtToEBITDA = nil
	part := Compute(in)
	if part.Scoreable != full.Scoreable-10 || part.Earned != full.Earned-8 {
		t.Errorf("scoreable %v earned %v (full %v/%v)", part.Scoreable, part.Earned, full.Scoreable, full.Earned)
	}
	if part.Safety == nil || *part.Safety <= part.Earned/full.Scoreable*100 {
		t.Errorf("the absence cost as much as a zero: %v", part.Safety)
	}
	var named bool
	for _, p := range part.Pillars {
		for _, u := range p.Unscored {
			named = named || strings.Contains(u, "net debt")
		}
	}
	if !named {
		t.Error("the unscored criterion is not named")
	}
}

func TestTooLittleScoreableIsNoScoreRatherThanALowOne(t *testing.T) {
	s := Compute(Input{PaysDividend: true, PayoutEarnings: pf(30), ScoreYear: 2026})
	if s.Safety != nil || !strings.Contains(s.Note, "could be scored") {
		t.Errorf("safety %v note %q", s.Safety, s.Note)
	}
}

func TestACompanyThatPaysNothingIsNotUnsafe(t *testing.T) {
	in := safePayer()
	in.PaysDividend = false
	if s := Compute(in); s.Safety != nil || s.Note != NoteNoDividend {
		t.Errorf("safety %v note %q", s.Safety, s.Note)
	}
}

func TestACutInTheLastTenYearsCapsTheScore(t *testing.T) {
	in := safePayer()
	in.CutYears = []int{2020}
	s := Compute(in)
	if s.Safety == nil || *s.Safety != CapCutCeiling || s.Band != "Unsafe" {
		t.Fatalf("safety %v band %q", s.Safety, s.Band)
	}
	if !s.Caps[0].Applied || s.Caps[0].Name != CapCut {
		t.Errorf("caps %+v", s.Caps)
	}
	for _, c := range Compute(safePayer()).Caps {
		if c.Applied {
			t.Errorf("%q bound a clean company", c.Name)
		}
	}
	// A cut under a board long gone is history.
	in.CutYears = []int{1991, 2002}
	if s := Compute(in); s.Safety == nil || *s.Safety == CapCutCeiling {
		t.Errorf("an old cut capped: %v", s.Safety)
	}
}

func TestAPayoutAboveBothNumeratorsCaps(t *testing.T) {
	in := safePayer()
	in.PayoutEarnings, in.PayoutFCF = pf(140), pf(120)
	if s := Compute(in); s.Safety == nil || *s.Safety != CapPayoutLimit {
		t.Fatalf("safety %v", s.Safety)
	}
	in = safePayer()
	in.PayoutEarnings = pf(140)
	if s := Compute(in); s.Safety == nil || *s.Safety <= CapPayoutLimit {
		t.Errorf("one numerator capped: %v", s.Safety)
	}
}

func TestAPropertyOwnerIsScoredOnFundsFromOperations(t *testing.T) {
	in := safePayer()
	in.REIT = true
	in.PayoutEarnings, in.PayoutFCF = pf(230), pf(-370)
	in.FCFCover, in.FCFMargin = pf(-0.27), pf(-61)
	in.ROIC, in.NetDebtToEBITDA = pf(3.4), pf(5.4)
	in.PayoutFFO, in.PayoutAFFO = pf(75.4), pf(75.2)
	in.FFOCover, in.FFOMargin = pf(100/75.4), pf(9.69)
	s := Compute(in)
	if s.Basis != BasisFundsFromOps || s.Safety == nil || *s.Safety < 55 {
		t.Errorf("basis %q safety %v note %q", s.Basis, s.Safety, s.Note)
	}
	// A REIT with only FFO (no AFFO source) is still scored: its floor is lower.
	in.PayoutAFFO = nil
	if s := Compute(in); s.Safety == nil {
		t.Errorf("FFO-only REIT not scored: %q", s.Note)
	}
}

func TestTheTenYearRecord(t *testing.T) {
	if !Compute(safePayer()).TenYearRecord {
		t.Error("thirty years did not clear ten")
	}
	in := safePayer()
	in.PaymentYears = pf(4)
	if Compute(in).TenYearRecord {
		t.Error("four years cleared ten")
	}
	in.PaymentYears = nil
	if Compute(in).TenYearRecord {
		t.Error("an unknown record cleared ten")
	}
}

func TestTheBands(t *testing.T) {
	for _, c := range []struct {
		at   float64
		want string
	}{{100, "Very Safe"}, {81, "Very Safe"}, {80, "Safe"}, {61, "Safe"}, {60, "Borderline"}, {41, "Borderline"},
		{40, "Unsafe"}, {21, "Unsafe"}, {20, "Very Unsafe"}, {0, "Very Unsafe"}} {
		if got := Band(c.at); got != c.want {
			t.Errorf("Band(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}

// --- the relative half ---

func scored(safety, cash, roe, yield, growth float64, years int, cs ...Cap) Member {
	s := Score{Safety: &safety, Band: Band(safety), Scoreable: 100, Earned: safety, TenYearRecord: years >= 10, Caps: cs,
		Factors: Factors{CashFlowToDebt: &cash, ReturnOnEquity: &roe, Yield: &yield, DividendGrowth5: &growth}}
	return Member{Score: s, Yield: yield}
}

func three() map[string]Member {
	return map[string]Member{
		"AAA": scored(90, 0.9, 40, 1.0, 12, 30),
		"BBB": scored(60, 0.4, 20, 4.0, 6, 20),
		"CCC": scored(30, 0.1, 8, 6.0, 1, 4),
	}
}

func TestAPercentileIsAboutTheSet(t *testing.T) {
	all := three()
	wide := RankSet(all)
	if wide.Companies != 3 || wide.Ranks["AAA"].Percentiles["cash flow to debt"] != 100 {
		t.Fatalf("%+v", wide.Ranks["AAA"])
	}
	narrow := RankSet(map[string]Member{"AAA": all["AAA"], "BBB": all["BBB"], "DDD": scored(95, 2.0, 60, 0.5, 20, 30)})
	if narrow.Ranks["AAA"].Percentiles["cash flow to debt"] == 100 {
		t.Error("ranked top beside a better one")
	}
	if *narrow.Ranks["AAA"].Safety != *wide.Ranks["AAA"].Safety {
		t.Error("the absolute half moved with the set")
	}
	if !strings.Contains(wide.Note, "3 companies") {
		t.Errorf("note %q", wide.Note)
	}
}

func TestTheCombinedFigureIsSixtyFortyAndCapsBindAfterIt(t *testing.T) {
	r := RankSet(three()).Ranks["AAA"]
	if r.Combined == nil || r.Composite == nil || *r.Combined != round2(0.6**r.Safety+0.4**r.Composite) {
		t.Fatalf("%+v", r)
	}
	all := three()
	all["AAA"] = scored(90, 0.9, 40, 1.0, 12, 30, Cap{Name: CapCut, Ceiling: CapCutCeiling, Applied: true})
	c := RankSet(all).Ranks["AAA"]
	if c.Combined == nil || *c.Combined != CapCutCeiling || c.CappedBy != CapCut || c.Band != "Unsafe" {
		t.Errorf("%+v", c)
	}
}

func TestTheScreenSaysWhichRuleWasMissed(t *testing.T) {
	s := RankSet(three())
	if s.Ranks["CCC"].Gate || !strings.Contains(s.Ranks["CCC"].GateWhy, "ten years") {
		t.Errorf("CCC %+v", s.Ranks["CCC"])
	}
	if s.Ranks["AAA"].Gate || !strings.Contains(s.Ranks["AAA"].GateWhy, "average") {
		t.Errorf("AAA %+v", s.Ranks["AAA"])
	}
	if !s.Ranks["BBB"].Gate {
		t.Errorf("BBB %q", s.Ranks["BBB"].GateWhy)
	}
	if s.AverageYield < 3.6 || s.AverageYield > 3.7 {
		t.Errorf("average %v", s.AverageYield)
	}
}

func TestTooSmallASetIsNotRanked(t *testing.T) {
	all := three()
	delete(all, "CCC")
	s := RankSet(all)
	if len(s.Ranks["AAA"].Percentiles) != 0 || s.Ranks["AAA"].Composite != nil || s.Ranks["AAA"].Combined != nil {
		t.Errorf("a set of two was ranked: %+v", s.Ranks["AAA"])
	}
	if RankSet(nil).Companies != 0 {
		t.Error("an empty read produced a set")
	}
}
