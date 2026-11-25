// Package rules is the investor's strategy in code (data/profile/strategy.yaml, pack.Strategy):
// the dividend groups, the safe-dividend checks, cap bands and the withholding at the active
// country and broker packs. Every limit is the investor's own; an unset one skips its rule.
// It is pure: inputs in, verdicts out, no I/O.
package rules

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/screen"
)

// The check outcomes. Missing data is NeedsLook, never a pass or a fail.
const (
	Pass      = "pass"
	Fail      = "fail"
	NeedsLook = "needs_look"
	Skipped   = "skipped"
)

// The groups, in the order a name's primary group is chosen.
const (
	GOAT         = "GOAT"
	Yielder      = "Yielder"
	Grower       = "Grower"
	StableGrower = "Stable Grower"
	StablePayer  = "Stable Payer"
	Growth       = "Growth"
)

// The cap bands, in millions of USD.
const (
	Mega  = "mega"
	Large = "large"
	Mid   = "mid"
	Small = "small"
)

// The cap bands' lower bounds, in millions of USD: standard sizes, not a strategy. Every other
// threshold is the investor's own (pack.Strategy, data/profile/strategy.yaml) and has no default.
const (
	MegaCapUSDMillions = 200_000.0
	LargeCapUSDMillion = 10_000.0
	MidCapUSDMillions  = 2_000.0
)

// Input is what one name is judged on. Percent fields are percent; nil is unknown.
type Input struct {
	Ticker   string
	ISIN     string
	Country  string // alpha-2 of the payer
	Sector   string
	Industry string

	YieldPct                     *float64
	Growth1y, Growth3y, Growth5y *float64
	Growth10y                    *float64
	PayoutPct                    *float64 // dividends over earnings
	FCFPayoutPct, OCFPayoutPct   *float64
	FFOPayoutPct                 *float64 // REITs, from EDGAR's FFO where known
	DebtToFCF                    *float64
	MarketCapUSDm                *float64
	PaymentYears, NoCutYears     int
	IncreaseYears                int
	// NoCutAtLeast and IncreaseAtLeast mark a streak the ledger cannot close (pb derive's
	// streak_at_least): a short streak is then "at least N" and a Stable group needs a look.
	NoCutAtLeast, IncreaseAtLeast bool
	HasDividendHistory            bool
	// EarningsLoss marks trailing-twelve-month earnings at or below zero: the earnings payout is
	// "n/a (loss)" and the payout rule is judged on cash flow. EarningsNote adds what the quarters
	// show (a one-off loss quarter).
	EarningsLoss bool
	EarningsNote string
}

// Check is one rule's outcome.
type Check struct {
	Name   string   `yaml:"name" json:"name"`
	Status string   `yaml:"status" json:"status"`
	Value  *float64 `yaml:"value,omitempty" json:"value,omitempty"`
	Limit  string   `yaml:"limit,omitempty" json:"limit,omitempty"`
	Note   string   `yaml:"note,omitempty" json:"note,omitempty"`
}

// Withholding is the active broker's rate for the payer, and its tier (a reading label: S 0%,
// A under 15%, B under 30%, C 30% and over; no rule passes or fails on it).
type Withholding struct {
	Country string  `yaml:"country" json:"country"`
	RatePct float64 `yaml:"rate_pct" json:"rate_pct"`
	Tier    string  `yaml:"tier" json:"tier"`
}

// Verdict is everything the rules say about one name.
type Verdict struct {
	Group       string       `yaml:"group" json:"group"`
	Groups      []string     `yaml:"groups,omitempty" json:"groups,omitempty"`
	GroupBasis  string       `yaml:"group_basis,omitempty" json:"group_basis,omitempty"`
	GroupNotes  []string     `yaml:"group_notes,omitempty" json:"group_notes,omitempty"`
	REIT        bool         `yaml:"reit" json:"reit"`
	Financial   bool         `yaml:"financial" json:"financial"`
	CapBand     string       `yaml:"cap_band" json:"cap_band"`
	Withholding *Withholding `yaml:"withholding,omitempty" json:"withholding,omitempty"`
	NetYieldPct *float64     `yaml:"net_yield_pct,omitempty" json:"net_yield_pct,omitempty"`
	Safe        string       `yaml:"safe_dividend" json:"safe_dividend"`
	Checks      []Check      `yaml:"checks" json:"checks"`
}

// IsREIT reads a REIT off the sector and industry words the vendors use.
func IsREIT(sector, industry string) bool {
	s := strings.ToLower(sector + " " + industry)
	return strings.Contains(s, "reit") || strings.Contains(s, "real estate investment trust")
}

// IsFinancial is a bank, insurer or BDC: the names the cover and Debt/FCF checks skip.
func IsFinancial(sector, industry string) bool {
	s := strings.ToLower(sector + " " + industry)
	for _, w := range []string{"bank", "insur", "business development", "bdc", "thrift", "savings"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// CapBand bands a market cap in millions of USD; "" when unknown.
func CapBand(usdMillions *float64) string {
	switch {
	case usdMillions == nil:
		return ""
	case *usdMillions >= MegaCapUSDMillions:
		return Mega
	case *usdMillions >= LargeCapUSDMillion:
		return Large
	case *usdMillions >= MidCapUSDMillions:
		return Mid
	default:
		return Small
	}
}

// WithholdingFor is the rate the active broker keeps for a payer (the packs state it). A
// US-numbered line of a company from another country (an ADR) is rated at the issuer's
// country, not the listing's.
func WithholdingFor(country, isin string, reit bool) *Withholding {
	r := screen.Row{Country: country, ISIN: isin}
	if c := strings.ToUpper(strings.TrimSpace(country)); len(isin) >= 2 && strings.EqualFold(isin[:2], "US") && len(c) == 2 && c != "US" {
		r.Payer, r.Type = c, "ADR"
	}
	t := screen.TaxOf(r, reit)
	if t == nil {
		return nil
	}
	return &Withholding{Country: t.Country, RatePct: t.Rate, Tier: t.Tier}
}

// NetYield is the yield left after a withholding rate, in percent.
func NetYield(yieldPct *float64, w *Withholding) *float64 {
	if yieldPct == nil || w == nil {
		return nil
	}
	return round2(*yieldPct * (1 - w.RatePct/100))
}

// Evaluate applies the strategy's group, cap and safe-dividend rules to one name, then every
// check a private layer registered (RegisterNameCheck).
func Evaluate(in Input) Verdict {
	v := Verdict{REIT: IsREIT(in.Sector, in.Industry), Financial: IsFinancial(in.Sector, in.Industry)}
	v.CapBand = CapBand(in.MarketCapUSDm)
	v.Withholding = WithholdingFor(in.Country, in.ISIN, v.REIT)
	v.NetYieldPct = NetYield(in.YieldPct, v.Withholding)
	v.groups(in)
	v.safety(in)
	for _, f := range nameChecks {
		f(in, &v)
	}
	return v
}

var (
	nameChecks      []func(Input, *Verdict)
	portfolioChecks []func(Allocation, *Report)
)

// RegisterNameCheck adds a check every name's verdict runs (a private layer's own rule, from init).
func RegisterNameCheck(f func(in Input, v *Verdict)) { nameChecks = append(nameChecks, f) }

// RegisterPortfolioCheck adds a rule every portfolio check runs after the built-in ones (an
// account's own rule, say a yearly deposit limit), from a private layer's init.
func RegisterPortfolioCheck(f func(a Allocation, r *Report)) {
	portfolioChecks = append(portfolioChecks, f)
}

// Add appends a rule's outcome to a report (for a registered portfolio check).
func (r *Report) Add(name, status, detail string) { r.add(name, status, detail) }

// payer reports a name that pays a dividend at all.
func (in Input) payer() bool {
	return in.YieldPct != nil && *in.YieldPct > 0
}

// groupGrowth is the growth the groups are judged on: the 5-year CAGR, else the 3-year.
func (in Input) groupGrowth() (*float64, string) {
	if in.Growth5y != nil {
		return in.Growth5y, "5y dividend CAGR"
	}
	if in.Growth3y != nil {
		return in.Growth3y, "3y dividend CAGR (no 5y)"
	}
	return nil, ""
}

func (v *Verdict) groups(in Input) {
	if !in.payer() {
		v.Group, v.Groups = Growth, []string{Growth}
		v.GroupBasis = "no dividend"
		return
	}
	st := pack.Active().Strategy
	if st.GroupYieldPct == nil && st.GrowerGrowthPct == nil && st.StableGrowerYears == nil && st.StablePayerYears == nil && st.LowYieldPct == nil {
		v.Group, v.GroupBasis = "none", "groups "+pack.UnsetNote
		return
	}
	y := *in.YieldPct
	g, basis := in.groupGrowth()
	v.GroupBasis = basis
	largeOnly := v.CapBand == Mega || v.CapBand == Large
	add := func(name string, _ bool) {
		needsLarge := slices.ContainsFunc(st.LargeCapOnlyGroups, func(g string) bool { return strings.EqualFold(strings.TrimSpace(g), name) })
		if needsLarge && !largeOnly {
			note := name + " needs a mega or large cap"
			if v.CapBand == "" {
				note += " (cap unknown: needs a look)"
			} else {
				note += " (is " + v.CapBand + ")"
			}
			v.GroupNotes = append(v.GroupNotes, note)
			return
		}
		v.Groups = append(v.Groups, name)
	}
	if g == nil {
		v.GroupNotes = append(v.GroupNotes, "no 3y or 5y dividend growth: GOAT and Grower need a look")
	}
	set := func(p *float64) bool { return p != nil }
	if set(st.GroupYieldPct) && y >= *st.GroupYieldPct && g != nil && set(st.GOATGrowthPct) && *g >= *st.GOATGrowthPct {
		add(GOAT, false)
	}
	if set(st.GroupYieldPct) && y >= *st.GroupYieldPct {
		add(Yielder, false)
	}
	if g != nil && set(st.GrowerGrowthPct) && *g >= *st.GrowerGrowthPct {
		add(Grower, true)
	}
	if st.StableGrowerYears != nil {
		if in.IncreaseYears >= *st.StableGrowerYears {
			add(StableGrower, true)
		} else if in.IncreaseAtLeast {
			v.GroupNotes = append(v.GroupNotes, fmt.Sprintf("%s: the increase streak is at least %d years (the ledger cannot close it): needs a look", StableGrower, in.IncreaseYears))
		}
	}
	if st.StablePayerYears != nil {
		if in.NoCutYears >= *st.StablePayerYears {
			add(StablePayer, true)
		} else if in.NoCutAtLeast {
			v.GroupNotes = append(v.GroupNotes, fmt.Sprintf("%s: the no-cut streak is at least %d years (the ledger cannot close it): needs a look", StablePayer, in.NoCutYears))
		}
	}
	if len(v.Groups) == 0 && set(st.LowYieldPct) && y < *st.LowYieldPct {
		v.Groups = []string{Growth}
	}
	if len(v.Groups) > 0 {
		v.Group = v.Groups[0]
	} else {
		v.Group = "none"
	}
}

func (v *Verdict) safety(in Input) {
	if !in.payer() {
		v.Safe = Skipped
		v.Checks = []Check{{Name: "safe_dividend", Status: Skipped, Note: "no dividend"}}
		return
	}
	for _, c := range []struct {
		name string
		val  *float64
	}{{"growth_1y", in.Growth1y}, {"growth_3y", in.Growth3y}, {"growth_5y", in.Growth5y}} {
		v.add(atLeast(c.name, c.val, 0, ">= 0%", "no negative dividend growth; rule out specials, short ledgers and FX before calling a cut"))
	}
	st := pack.Active().Strategy
	unset := st.MaxPayoutPct == nil && st.MaxREITPayoutPct == nil && st.MaxDebtToFCF == nil && st.MinCover == nil
	if unset {
		v.add(Check{Name: "safe_dividend_limits", Status: Skipped, Note: "payout, debt and cover limits " + pack.UnsetNote})
	} else {
		v.add(v.payout(in))
		v.add(v.debtToFCF(in))
		v.add(v.cover(in))
	}
	// A failed growth check fails the name even with no limits set; with nothing but the growth
	// checks passing, the verdict stays skipped (the limits were never judged).
	v.Safe = Pass
	if unset {
		v.Safe = Skipped
	}
	for _, c := range v.Checks {
		switch c.Status {
		case Fail:
			v.Safe = Fail
			return
		case NeedsLook:
			v.Safe = NeedsLook
		}
	}
}

func (v *Verdict) add(c Check) { v.Checks = append(v.Checks, c) }

// unsetCheck is a check whose limit the strategy does not state: skipped, and saying so.
func unsetCheck(name, what string) Check {
	return Check{Name: name, Status: Skipped, Note: what + " " + pack.UnsetNote}
}

func (v *Verdict) payout(in Input) Check {
	st := pack.Active().Strategy
	if v.REIT {
		if st.MaxREITPayoutPct == nil {
			return unsetCheck("payout", "the REIT payout limit (max_reit_payout_pct) is")
		}
		lim := *st.MaxREITPayoutPct
		switch {
		case in.FFOPayoutPct != nil:
			return below("payout", in.FFOPayoutPct, lim, fmt.Sprintf("< %g%% of FFO", lim), "REIT, on EDGAR FFO")
		case in.OCFPayoutPct != nil:
			return below("payout", in.OCFPayoutPct, lim, fmt.Sprintf("< %g%% of OCF", lim), "REIT, on operating cash flow (no FFO)")
		}
		return Check{Name: "payout", Status: NeedsLook, Limit: fmt.Sprintf("< %g%% of FFO or OCF", lim), Note: "REIT with no FFO or OCF payout"}
	}
	if st.MaxPayoutPct == nil {
		return unsetCheck("payout", "the payout limit (max_payout_pct) is")
	}
	if in.EarningsLoss || (in.PayoutPct != nil && *in.PayoutPct < 0) {
		return lossPayout(in, *st.MaxPayoutPct)
	}
	return below("payout", in.PayoutPct, *st.MaxPayoutPct, fmt.Sprintf("< %g%% of earnings", *st.MaxPayoutPct), "")
}

// PayoutLoss is how the earnings payout reads when trailing earnings are at or below zero.
const PayoutLoss = "n/a (loss)"

// lossPayout judges a loss-maker's payout on free cash flow, else operating cash flow, against the
// strategy's payout limit: a payout over negative earnings is not a ratio anyone can read.
func lossPayout(in Input, lim float64) Check {
	note := "earnings payout " + PayoutLoss + ": trailing-twelve-month earnings at or below zero"
	if in.EarningsNote != "" {
		note += "; " + in.EarningsNote
	}
	switch {
	case in.FCFPayoutPct != nil && *in.FCFPayoutPct > 0:
		c := below("payout", in.FCFPayoutPct, lim, fmt.Sprintf("< %g%% of free cash flow (loss year)", lim), note+"; judged on the free-cash-flow payout")
		return c
	case in.OCFPayoutPct != nil && *in.OCFPayoutPct > 0:
		return below("payout", in.OCFPayoutPct, lim, fmt.Sprintf("< %g%% of operating cash flow (loss year)", lim), note+"; judged on the operating-cash-flow payout (no FCF payout)")
	}
	return Check{Name: "payout", Status: NeedsLook, Limit: fmt.Sprintf("< %g%% of free or operating cash flow (loss year)", lim), Note: note + "; no positive cash-flow payout to judge it on"}
}

func (v *Verdict) debtToFCF(in Input) Check {
	switch {
	case v.Financial:
		return Check{Name: "debt_to_fcf", Status: Skipped, Value: in.DebtToFCF, Note: "bank, insurer or BDC"}
	case v.CapBand == Mega || v.CapBand == Large:
		return Check{Name: "debt_to_fcf", Status: Skipped, Value: in.DebtToFCF, Note: v.CapBand + " cap is exempt"}
	}
	st := pack.Active().Strategy
	if st.MaxDebtToFCF == nil {
		return unsetCheck("debt_to_fcf", "the Debt/FCF limit (max_debt_to_fcf) is")
	}
	c := below("debt_to_fcf", in.DebtToFCF, *st.MaxDebtToFCF, fmt.Sprintf("< %g", *st.MaxDebtToFCF), "")
	if in.DebtToFCF != nil && *in.DebtToFCF < 0 {
		c.Status, c.Note = Fail, "negative: free cash flow below zero"
	}
	if v.CapBand == "" && c.Status == Fail {
		c.Status, c.Note = NeedsLook, "cap band unknown, so the large-cap exemption cannot be ruled out"
	}
	return c
}

func (v *Verdict) cover(in Input) Check {
	if v.Financial {
		return Check{Name: "cover", Status: Skipped, Note: "bank, insurer or BDC"}
	}
	payout, basis := in.FCFPayoutPct, "free cash flow"
	if v.REIT {
		payout, basis = in.OCFPayoutPct, "operating cash flow (REIT)"
	}
	st := pack.Active().Strategy
	if st.MinCover == nil {
		return unsetCheck("cover", "the cover minimum (min_cover) is")
	}
	lim := fmt.Sprintf(">= %gx", *st.MinCover)
	if payout == nil || *payout == 0 {
		return Check{Name: "cover", Status: NeedsLook, Limit: lim, Note: "no " + basis + " payout"}
	}
	if *payout < 0 {
		return Check{Name: "cover", Status: Fail, Limit: lim, Note: basis + " is negative"}
	}
	cover := round2(100 / *payout)
	c := atLeast("cover", cover, *st.MinCover, lim, "dividend covered by "+basis)
	return c
}

func below(name string, val *float64, limit float64, label, note string) Check {
	c := Check{Name: name, Value: val, Limit: label, Note: note}
	switch {
	case val == nil:
		c.Status = NeedsLook
	case *val < limit:
		c.Status = Pass
	default:
		c.Status = Fail
	}
	return c
}

func atLeast(name string, val *float64, limit float64, label, note string) Check {
	c := Check{Name: name, Value: val, Limit: label}
	switch {
	case val == nil:
		c.Status = NeedsLook
	case *val >= limit:
		c.Status = Pass
	default:
		c.Status, c.Note = Fail, note
	}
	return c
}

func round2(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	r := math.Round(v*100) / 100
	return &r
}
