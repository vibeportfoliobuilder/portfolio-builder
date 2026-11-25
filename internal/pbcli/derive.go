package pbcli

// derive.go - pb derive: the profile's rules over the fetched files, into derived.yaml.

import (
	"cmp"
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/divscore"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/returns"
)

// Derived is data/stocks/<T>/derived.yaml: what the rules say about one name, with the
// figures every name shows.
type Derived struct {
	AsOf             string `yaml:"as_of" json:"as_of"`
	Ticker           string `yaml:"ticker" json:"ticker"`
	ISIN             string `yaml:"isin,omitempty" json:"isin,omitempty"`
	Name             string `yaml:"name" json:"name"`
	Sector           string `yaml:"sector,omitempty" json:"sector,omitempty"`
	Industry         string `yaml:"industry,omitempty" json:"industry,omitempty"`
	Country          string `yaml:"country" json:"country"`
	DividendCurrency string `yaml:"dividend_currency,omitempty" json:"dividend_currency,omitempty"`
	// The currency the company declares, and why dividend_currency (what the broker credits) differs.
	DividendCurrencyDeclared string              `yaml:"dividend_currency_declared,omitempty" json:"dividend_currency_declared,omitempty"`
	DividendCurrencyBasis    string              `yaml:"dividend_currency_basis,omitempty" json:"dividend_currency_basis,omitempty"`
	Price                    *float64            `yaml:"price,omitempty" json:"price,omitempty"`
	PriceCurrency            string              `yaml:"price_currency,omitempty" json:"price_currency,omitempty"`
	MarketCapUSDm            *float64            `yaml:"market_cap_usd_m,omitempty" json:"market_cap_usd_m,omitempty"`
	YieldPct                 *float64            `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	DividendGrowth           map[string]*float64 `yaml:"dividend_growth_pct" json:"dividend_growth_pct"`
	PayoutPct                *float64            `yaml:"payout_pct,omitempty" json:"payout_pct,omitempty"`
	// PayoutNote explains an absent or corrected payout: "n/a (loss)", a one-off loss quarter.
	PayoutNote   string              `yaml:"payout_note,omitempty" json:"payout_note,omitempty"`
	FCFPayoutPct *float64            `yaml:"fcf_payout_pct,omitempty" json:"fcf_payout_pct,omitempty"`
	OCFPayoutPct *float64            `yaml:"ocf_payout_pct,omitempty" json:"ocf_payout_pct,omitempty"`
	FFOPayoutPct *float64            `yaml:"ffo_payout_pct,omitempty" json:"ffo_payout_pct,omitempty"`
	DebtToFCF    *float64            `yaml:"debt_to_fcf,omitempty" json:"debt_to_fcf,omitempty"`
	PE           *float64            `yaml:"pe,omitempty" json:"pe,omitempty"`
	PEGY         *float64            `yaml:"pegy,omitempty" json:"pegy,omitempty"`
	PriceCAGR    map[string]*float64 `yaml:"price_cagr_pct" json:"price_cagr_pct"`
	// TotalReturn reinvests dividends gross of withholding (pb ep returns); NA says why a figure is absent.
	TotalReturn    map[string]*float64 `yaml:"total_return_pct,omitempty" json:"total_return_pct,omitempty"`
	NA             map[string]string   `yaml:"n_a,omitempty" json:"n_a,omitempty"`
	ReturnsAsOf    string              `yaml:"returns_as_of,omitempty" json:"returns_as_of,omitempty"`
	DividendSafety *divscore.Score     `yaml:"dividend_safety,omitempty" json:"dividend_safety,omitempty"`
	// SpecialDividends flags relatively large special payments (pb ep returns' 5 years, else the dossier's 3).
	SpecialDividends *returns.Special `yaml:"special_dividends,omitempty" json:"special_dividends,omitempty"`
	Streaks          map[string]int   `yaml:"streak_years" json:"streak_years"`
	// StreakAtLeast names the streaks the ledger cannot close ("at least N"), with the reason.
	StreakAtLeast   map[string]string `yaml:"streak_at_least,omitempty" json:"streak_at_least,omitempty"`
	HumanView       *HumanViewSummary `yaml:"human_view,omitempty" json:"human_view,omitempty"`
	BrokerConsensus *TargetSummary    `yaml:"broker_consensus,omitempty" json:"broker_consensus,omitempty"`
	Rules           rules.Verdict     `yaml:"rules" json:"rules"`
	// TotalReturnFlags are R1b and R3: a "flag" needs a stated reason to hold, not an exclusion.
	TotalReturnFlags []rules.Check `yaml:"total_return_flags,omitempty" json:"total_return_flags,omitempty"`
	DataGaps         []string      `yaml:"data_gaps,omitempty" json:"data_gaps,omitempty"`
	// YoCPath is the name's own net yield on cost, years 1-10, on the dividend growth pb projects
	// (the strategy's haircut and cap): no reinvestment and no new money, so the scenario analyst
	// cites it as the base path instead of compounding by hand.
	YoCPath *NameYoCPath `yaml:"yoc_path,omitempty" json:"yoc_path,omitempty"`
	// MaxDrawdownPct is returns.yaml's worst month-end peak-to-trough fall (negative, percent).
	MaxDrawdownPct *float64 `yaml:"max_drawdown_pct,omitempty" json:"max_drawdown_pct,omitempty"`
	// TTMUsed is the trailing-twelve-month block after pb repair, present only when a repair changed
	// it (data_gaps says which); it replaces financial_data.yaml's ttm.statements for this name.
	TTMUsed *eulerpool.Statements `yaml:"ttm_used,omitempty" json:"ttm_used,omitempty"`
}

// HumanViewSummary is human_view.yaml's headline, with the price against fair value.
type HumanViewSummary struct {
	Moat             string   `yaml:"moat,omitempty" json:"moat,omitempty"`
	MoatTrend        string   `yaml:"moat_trend,omitempty" json:"moat_trend,omitempty"`
	FairValue        float64  `yaml:"fair_value,omitempty" json:"fair_value,omitempty"`
	Currency         string   `yaml:"currency,omitempty" json:"currency,omitempty"`
	PriceToFairValue *float64 `yaml:"price_to_fair_value,omitempty" json:"price_to_fair_value,omitempty"`
	// FXNote cites the rates when the price and the fair value are in different currencies.
	FXNote      string `yaml:"fx_note,omitempty" json:"fx_note,omitempty"`
	Uncertainty string `yaml:"uncertainty,omitempty" json:"uncertainty,omitempty"`
}

// TargetSummary is the broker's analyst consensus (broker.yaml), a second human view.
type TargetSummary struct {
	TargetMean float64 `yaml:"target_mean,omitempty" json:"target_mean,omitempty"`
	Analysts   int     `yaml:"analysts,omitempty" json:"analysts,omitempty"`
	RatingMean float64 `yaml:"rating_mean,omitempty" json:"rating_mean,omitempty"`
	RatingDate string  `yaml:"rating_date,omitempty" json:"rating_date,omitempty"`
}

func deriveCmd(e *env) *cobra.Command {
	var watch bool
	c := &cobra.Command{
		Use:   "derive <TICKER ...> | --watchlist",
		Short: "Apply the profile's rules: group, safe-dividend checks, withholding, net yield, cap band",
		Long: `Reads data/stocks/<T>/financial_data.yaml (required; pb ep fetch) plus, where present,
broker.yaml, human_view.yaml, edgar.yaml and data/macro.yaml (FX for the USD market cap), and writes
data/stocks/<T>/derived.yaml. No network.

Rules: YOUR strategy (data/profile/strategy.yaml; ./pb packs prints it). Each limit is a strategy key;
an unset key skips its rule ("skipped", saying which key), never a default:
  groups   GOAT yield >= group_yield_pct and growth >= goat_growth_pct; Yielder yield >= group_yield_pct;
           Grower growth >= grower_growth_pct; Stable Grower stable_grower_years of increases; Stable
           Payer stable_payer_years with no cut; Growth no dividend or yield < low_yield_pct. Growth is
           the 5y dividend CAGR (3y when no 5y). large_cap_only_groups lists the groups a mid or small
           cap cannot join.
  flags    total return (a flag needs a reason to hold, never an exclusion): a yield >= high_yield_flag_pct
           needs a 10y price CAGR >= high_yield_min_price_cagr_pct; net yield + 5y dividend CAGR >=
           min_yield_plus_growth_pct; missing data "needs_look".
  safety   no negative dividend growth over 1, 3 or 5 years (always checked); payout < max_payout_pct
           (REIT < max_reit_payout_pct of FFO, else OCF); Debt/FCF < max_debt_to_fcf except large and
           mega caps; cover >= min_cover (FCF; OCF for REITs); banks, insurers and BDCs skip cover and
           Debt/FCF. Missing data is "needs_look", never a pass or a fail.
  tax      the withholding the country and broker packs state (pb packs); tiers S/A/B/C are labels.
  caps     mega >=$200B, large $10–200B, mid $2–10B, small <$2B (standard bands).
Examples:
  pb derive KO PEP
  pb derive --watchlist --md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ts, err := e.targets(args, watch, kindStock)
			if err != nil {
				return err
			}
			fx := e.fxTable()
			var out []Derived
			var errs []string
			for _, t := range ts {
				d, err := e.derive(t, fx)
				if err != nil {
					errs = append(errs, t.Ticker+": "+err.Error())
					continue
				}
				if _, err := e.writeYAML(d, "stocks", t.Ticker, "derived.yaml"); err != nil {
					return err
				}
				out = append(out, d)
			}
			if e.md {
				return e.deriveMD(out, errs)
			}
			return e.printJSON(map[string]any{"derived": out, "errors": errs})
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "every stock in data/watchlist.yaml")
	return c
}

func (e *env) derive(t target, fx fxTable) (Derived, error) {
	var d eulerpool.Dossier
	ok, err := e.readYAML(&d, "stocks", t.Ticker, "financial_data.yaml")
	if err != nil {
		return Derived{}, err
	}
	if !ok {
		return Derived{}, fmt.Errorf("no financial_data.yaml: run pb ep fetch %s", t.Ticker)
	}
	var ly broker.Stock
	lyOK, _ := e.readYAML(&ly, "stocks", t.Ticker, broker.File)
	var repairs []string
	lyPayout := 0.0
	if lyOK && ly.Company != nil {
		lyPayout = ly.Company.PayoutRatioPct
	}
	if n := unsplitLedger(&d, lyPayout); n != "" {
		repairs = append(repairs, n)
	}
	// Specials out of the growth and the streaks (on the ledger as repaired above, so an unadjusted
	// split cannot pass for a special).
	if d.Dividends != nil {
		var ps []returns.Payment
		for _, p := range d.Dividends.Payments {
			ps = append(ps, returns.Payment{ExDate: p.ExDate, Amount: p.DPS})
		}
		if sp := returns.Specials(ps, e.now().UTC()); sp != nil {
			if n := eulerpool.DropSpecials(&d, sp.ExDates); n != "" {
				repairs = append(repairs, n)
			}
		}
	}
	before := d.TTM
	if before != nil {
		c := *before
		before = &c
	}
	repairs = append(repairs, eulerpool.Repair(&d)...)
	d.DataGaps = append(slices.Clone(d.DataGaps), repairs...)
	out := Derived{AsOf: e.today(), Ticker: t.Ticker, ISIN: d.ISIN, Name: d.Identity.Name, Sector: d.Identity.Sector,
		Industry: d.Identity.Industry, Country: strings.ToUpper(d.Identity.Country), Price: d.Price.Current,
		PriceCurrency: d.PriceCurrency, PE: d.Ratios.PE, PEGY: d.Ratios.PEGY, PayoutPct: d.Ratios.PayoutRatio,
		FCFPayoutPct: d.Ratios.FCFPayoutRatio, OCFPayoutPct: d.Ratios.OCFPayoutRatio, DebtToFCF: d.Ratios.DebtToFCF,
		PriceCAGR:      map[string]*float64{"1y": d.Price.CAGR1y, "5y": d.Price.CAGR5y, "10y": d.Price.CAGR10y},
		DividendGrowth: map[string]*float64{}, Streaks: map[string]int{}, DataGaps: d.DataGaps}
	// The repaired TTM, for the analysts who would otherwise read financial_data.yaml's raw block.
	if d.TTM != nil && before != nil && !sameStatements(d.TTM.Statements, before.Statements) {
		st := d.TTM.Statements
		out.TTMUsed = &st
	}
	// An ADR's dossier names the US listing's country; the watchlist (from the screener) names the issuer's.
	if it := t.Item; it != nil && strings.HasPrefix(strings.ToUpper(d.ISIN), "US") && len(it.Country) == 2 && !strings.EqualFold(it.Country, out.Country) && !strings.EqualFold(it.Country, "US") {
		out.DataGaps = append(out.DataGaps, fmt.Sprintf("pb derive: US-numbered line of a %s company (ADR): country and withholding follow the issuer, not the listing's %s", strings.ToUpper(it.Country), out.Country))
		out.Country = strings.ToUpper(it.Country)
	}
	in := rules.Input{Ticker: t.Ticker, ISIN: d.ISIN, Country: out.Country, Sector: d.Identity.Sector, Industry: d.Identity.Industry,
		PayoutPct: d.Ratios.PayoutRatio, FCFPayoutPct: d.Ratios.FCFPayoutRatio, OCFPayoutPct: d.Ratios.OCFPayoutRatio,
		DebtToFCF: d.Ratios.DebtToFCF, YieldPct: d.Ratios.DividendYield}
	if loss, note := earningsLoss(d); loss {
		in.EarningsLoss, in.EarningsNote = true, note
		out.PayoutPct, in.PayoutPct = nil, nil
		out.PayoutNote = rules.PayoutLoss + ": trailing-twelve-month earnings at or below zero; safety is judged on the cash-flow payout"
		if note != "" {
			out.PayoutNote += "; " + note
		}
	}
	if h := d.Dividends; h != nil {
		out.DividendCurrency = normCcy(h.Currency)
		if h.Yield != nil {
			in.YieldPct = h.Yield
		}
		in.Growth1y, in.Growth3y, in.Growth5y, in.Growth10y = h.CAGR1y, h.CAGR3y, h.CAGR5y, h.CAGR10y
		in.PaymentYears, in.NoCutYears, in.IncreaseYears = h.PaymentYears, h.NoCutYears, h.IncreaseStreakYears
		in.HasDividendHistory = true
		out.Streaks = map[string]int{"payment": h.PaymentYears, "no_cut": h.NoCutYears, "increase": h.IncreaseStreakYears}
		if fl := eulerpool.StreakFloors(h); fl != nil {
			out.StreakAtLeast = fl
			in.NoCutAtLeast, in.IncreaseAtLeast = fl["no_cut"] != "", fl["increase"] != ""
			for _, k := range []string{"payment", "no_cut", "increase"} {
				if fl[k] != "" {
					out.DataGaps = append(out.DataGaps, fmt.Sprintf("pb derive: the %s streak of %d years %s: read it as at least %d", strings.ReplaceAll(k, "_", "-"), out.Streaks[k], fl[k], out.Streaks[k]))
				}
			}
		}
	}
	if d.Dividends == nil {
		if y, g, note := cashDividend(d); y != nil {
			in.YieldPct, in.HasDividendHistory = y, true
			in.Growth1y, in.Growth3y, in.Growth5y = g["1y"], g["3y"], g["5y"]
			out.DividendCurrency = normCcy(cmp.Or(d.PriceCurrency, d.Currency))
			out.DataGaps = append(out.DataGaps, note)
		}
	}
	out.DividendGrowth = map[string]*float64{"1y": in.Growth1y, "3y": in.Growth3y, "5y": in.Growth5y, "10y": in.Growth10y}
	out.YieldPct = in.YieldPct
	if d.Price.MarketCap != nil {
		if v, ok := fx.toUSD(*d.Price.MarketCap, normCcy(d.Currency)); ok {
			out.MarketCapUSDm = rounded(v, 0)
			in.MarketCapUSDm = out.MarketCapUSDm
		} else {
			out.DataGaps = append(out.DataGaps, "pb derive: no FX rate for "+d.Currency+" in data/macro.yaml, so no USD market cap or cap band (run pb ep macro)")
		}
	}
	var ed EdgarDoc
	var edp *EdgarDoc
	if ok, _ := e.readYAML(&ed, "stocks", t.Ticker, "edgar.yaml"); ok {
		out.FFOPayoutPct = ffoPayout(d, ed)
		in.FFOPayoutPct = out.FFOPayoutPct
		edp = &ed
	}
	var hv HumanView
	if ok, _ := e.readYAML(&hv, "stocks", t.Ticker, "human_view.yaml"); ok && hv.IsCovered() {
		m := &HumanViewSummary{Moat: hv.Moat, MoatTrend: hv.MoatTrend, FairValue: hv.FairValue, Currency: hv.FairValueCurrency, Uncertainty: hv.Uncertainty}
		if hv.FairValue > 0 && d.Price.Current != nil {
			if p, note, ok := fx.priceToFairValue(*d.Price.Current, d.PriceCurrency, hv.FairValue, hv.FairValueCurrency); ok {
				m.PriceToFairValue, m.FXNote = p, note
			} else {
				out.DataGaps = append(out.DataGaps, note)
			}
		}
		out.HumanView = m
	}
	if lyOK && ly.Company != nil {
		co := ly.Company
		out.BrokerConsensus = &TargetSummary{TargetMean: co.TargetMean, Analysts: co.TargetAnalysts, RatingMean: co.RatingMean, RatingDate: co.RatingDate}
		if out.DividendCurrency == "" {
			out.DividendCurrency = normCcy(ly.Instrument.Currency)
		}
	}
	e.creditedCurrency(&out, t, ly)
	out.Rules = rules.Evaluate(in)
	e.deriveReturns(&out, t.Ticker)
	// No yield and no dividend history is no dividend (as for the n_a reasons), so R1b/R3 do not apply.
	trYield := out.YieldPct
	if trYield == nil && !in.HasDividendHistory {
		trYield = new(float64)
	}
	out.TotalReturnFlags = rules.TotalReturnFlags(trYield, out.Rules.NetYieldPct, out.PriceCAGR["10y"], out.DividendGrowth["5y"])
	out.YoCPath = nameYoCPath(out.Rules.NetYieldPct, out.DividendGrowth)
	if out.SpecialDividends == nil && d.Dividends != nil {
		var ps []returns.Payment
		for _, p := range d.Dividends.Payments {
			ps = append(ps, returns.Payment{ExDate: p.ExDate, Amount: p.DPS})
		}
		out.SpecialDividends = returns.Specials(ps, e.now().UTC())
	}
	sc := divscore.Compute(scoreInput(d, out, edp, e.benchCloses(), e.now().UTC()))
	out.DividendSafety = &sc
	return out, nil
}

// deriveReturns adds pb ep returns' total return, and its price CAGRs where the dossier has none;
// without returns.yaml the total return is "not fetched".
func (e *env) deriveReturns(out *Derived, ticker string) {
	var f returns.Figures
	ok, _ := e.readYAML(&f, "stocks", ticker, "returns.yaml")
	out.NA = map[string]string{}
	if ok {
		out.MaxDrawdownPct = f.MaxDrawdownPct
	}
	for _, y := range returns.Horizons {
		k := returns.Key(y)
		if ok && out.PriceCAGR[k] == nil && f.PriceCAGR[k] != nil {
			out.PriceCAGR[k] = f.PriceCAGR[k]
		}
		if out.PriceCAGR[k] == nil {
			out.NA["price_cagr_pct."+k] = cmp.Or(f.NA["price_cagr_pct."+k], returns.NoHistory)
		}
		if out.DividendGrowth[k] == nil {
			why := f.NA["dividend_growth_pct."+k]
			if out.YieldPct == nil || *out.YieldPct == 0 {
				why = returns.NoDividend
			}
			out.NA["dividend_growth_pct."+k] = cmp.Or(why, returns.NoHistory)
		}
		if !ok {
			out.NA["total_return_pct."+k] = returns.NotFetched
			continue
		}
		if out.TotalReturn == nil {
			out.TotalReturn = map[string]*float64{}
		}
		if v := f.TotalReturn[k]; v != nil {
			out.TotalReturn[k] = v
		} else {
			out.NA["total_return_pct."+k] = cmp.Or(f.NA["total_return_pct."+k], returns.NoHistory)
		}
	}
	if ok {
		out.ReturnsAsOf = f.AsOf
		out.SpecialDividends = f.Specials
	}
	if len(out.NA) == 0 {
		out.NA = nil
	}
}

// cashDividend is the dividend of a name with no ledger (Eulerpool has none for some
// ADRs): cash-flow dividends paid over the shares outstanding, the TTM for the yield and fiscal years
// for the growth. Per-share figures are per listed share (the ADR), in the statements' currency.
func cashDividend(d eulerpool.Dossier) (*float64, map[string]*float64, string) {
	if d.TTM == nil || d.TTM.Statements.DividendsPaid == nil || d.Shares.Outstanding == nil || *d.Shares.Outstanding <= 0 ||
		d.Price.Current == nil || *d.Price.Current <= 0 || !strings.EqualFold(normCcy(d.Currency), normCcy(d.PriceCurrency)) {
		return nil, nil, ""
	}
	paid := math.Abs(*d.TTM.Statements.DividendsPaid)
	if paid == 0 {
		return nil, nil, ""
	}
	dps := paid / *d.Shares.Outstanding
	y := rounded(dps / *d.Price.Current * 100, 2)
	shares := map[int]float64{}
	for _, s := range d.Shares.ShareCountTrend {
		if len(s.Period) >= 4 && s.Shares > 0 {
			fy, _ := strconv.Atoi(s.Period[:4])
			shares[fy] = s.Shares
		}
	}
	perYear := map[int]float64{}
	last := 0
	for _, yr := range d.Years {
		fy := yr.FiscalYear
		if fy == 0 && len(yr.PeriodEnd) >= 4 {
			fy, _ = strconv.Atoi(yr.PeriodEnd[:4])
		}
		if yr.Statements.DividendsPaid == nil || shares[fy] <= 0 || *yr.Statements.DividendsPaid == 0 {
			continue
		}
		perYear[fy] = math.Abs(*yr.Statements.DividendsPaid) / shares[fy]
		last = max(last, fy)
	}
	g := map[string]*float64{}
	for _, n := range []int{1, 3, 5} {
		if a, b := perYear[last-n], perYear[last]; a > 0 && b > 0 {
			g[fmt.Sprintf("%dy", n)] = rounded((math.Pow(b/a, 1/float64(n))-1)*100, 2)
		}
	}
	return y, g, fmt.Sprintf("pb derive: no dividend ledger (an ADR at Eulerpool); yield %s%% is TTM cash dividends paid %s m / %s m shares = %s per share, growth from fiscal-year dividends paid per share",
		strconv.FormatFloat(*y, 'f', -1, 64), strconv.FormatFloat(round2v(paid), 'f', -1, 64), strconv.FormatFloat(*d.Shares.Outstanding, 'f', -1, 64), strconv.FormatFloat(round2v(dps), 'f', -1, 64))
}

// earningsLoss reports trailing-twelve-month earnings at or below zero (TTM net income, else a
// negative earnings payout), and whether the last four quarters show a single loss quarter among
// profitable ones: a one-off charge, not a business that has stopped earning.
func earningsLoss(d eulerpool.Dossier) (bool, string) {
	loss := false
	if d.TTM != nil && d.TTM.Statements.NetIncome != nil {
		loss = *d.TTM.Statements.NetIncome <= 0
	} else if d.Ratios.PayoutRatio != nil {
		loss = *d.Ratios.PayoutRatio < 0
	}
	if !loss {
		return false, ""
	}
	qs := distinctQuarters(d.Quarters)
	if len(qs) < 4 {
		return true, ""
	}
	qs = qs[len(qs)-4:]
	var neg []eulerpool.Period
	for _, q := range qs {
		if *q.Statements.NetIncome < 0 {
			neg = append(neg, q)
		}
	}
	if len(neg) == 1 {
		return true, fmt.Sprintf("one-off loss quarter %s (net income %s) among three profitable ones", neg[0].PeriodEnd,
			strconv.FormatFloat(*neg[0].Statements.NetIncome, 'f', -1, 64))
	}
	return true, ""
}

// distinctQuarters drops the vendor's repeated quarters (the same net income under a period end a
// few days apart) and quarters with no net income, oldest first.
func distinctQuarters(qs []eulerpool.Period) []eulerpool.Period {
	var out []eulerpool.Period
	for _, q := range qs {
		if q.Statements.NetIncome == nil {
			continue
		}
		if n := len(out); n > 0 && daysApart(out[n-1].PeriodEnd, q.PeriodEnd) <= 15 && *out[n-1].Statements.NetIncome == *q.Statements.NetIncome {
			out[n-1] = q
			continue
		}
		out = append(out, q)
	}
	return out
}

func daysApart(a, b string) int {
	ta, e1 := time.Parse("2006-01-02", a)
	tb, e2 := time.Parse("2006-01-02", b)
	if e1 != nil || e2 != nil {
		return 1 << 20
	}
	return int(math.Abs(tb.Sub(ta).Hours() / 24))
}

// ffoPayout is dividends paid over EDGAR FFO for the newest fiscal year both state.
func ffoPayout(d eulerpool.Dossier, ed EdgarDoc) *float64 {
	paid := map[int]float64{}
	for _, y := range d.Years {
		if y.Statements.DividendsPaid != nil && *y.Statements.DividendsPaid != 0 {
			fy := y.FiscalYear
			if fy == 0 && len(y.PeriodEnd) >= 4 {
				fy, _ = strconv.Atoi(y.PeriodEnd[:4])
			}
			paid[fy] = math.Abs(*y.Statements.DividendsPaid)
		}
	}
	for _, f := range ed.FFO { // newest first
		if len(f.PeriodEnd) < 4 || f.FFOm <= 0 {
			continue
		}
		fy, _ := strconv.Atoi(f.PeriodEnd[:4])
		if p, ok := paid[fy]; ok {
			return rounded(p/f.FFOm*100, 2)
		}
	}
	return nil
}

// normCcy folds a minor-unit code into its currency.
// minorUnit is what one unit of a quote currency is in its major currency: 0.01 for the quotes in
// cents (GBX/GBp pence, ZAc South African cents, ILA agorot), else 1.
func minorUnit(c string) float64 {
	switch strings.TrimSpace(c) {
	case "GBX", "GBx", "GBp", "ZAc", "ZAC", "ILA", "ILa", "GBPENCE":
		return 0.01
	}
	return 1
}

func normCcy(c string) string {
	switch c = strings.ToUpper(strings.TrimSpace(c)); c {
	case "GBX", "GBP.", "GBPENCE":
		return "GBP"
	case "ZAC":
		return "ZAR"
	case "ILA":
		return "ILS"
	}
	return c
}

func rounded(v float64, dp int) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	p := math.Pow(10, float64(dp))
	r := math.Round(v*p) / p
	return &r
}

func round2v(v float64) float64 { return math.Round(v*100) / 100 }

func (e *env) deriveMD(ds []Derived, errs []string) error {
	var rows [][]string
	for _, d := range ds {
		v := d.Rules
		tier, rate := "–", "–"
		if v.Withholding != nil {
			tier, rate = v.Withholding.Tier, strconv.FormatFloat(v.Withholding.RatePct, 'f', -1, 64)
		}
		moat, pfv := "–", "–"
		if d.HumanView != nil {
			moat = d.HumanView.Moat + " / " + d.HumanView.MoatTrend
			pfv = num(d.HumanView.PriceToFairValue, 2)
		}
		rows = append(rows, []string{d.Ticker, d.Name, d.Industry, d.Country + " " + tier + " " + rate + "%", v.CapBand, v.Group,
			num(d.YieldPct, 2), num(v.NetYieldPct, 2), num(d.DividendGrowth["1y"], 1) + " / " + num(d.DividendGrowth["3y"], 1) + " / " + num(d.DividendGrowth["5y"], 1),
			num(d.PayoutPct, 0), num(d.DebtToFCF, 1), num(d.PEGY, 2), moat, pfv, d.DividendCurrency, v.Safe, trFlagsText(d.TotalReturnFlags)})
	}
	s := mdTable([]string{"Ticker", "Name", "Industry", "Withholding", "Cap", "Group", "Yield %", "Net %", "Div g 1/3/5y %",
		"Payout %", "Debt/FCF", "PEGY", "Moat", "P/FV", "Div ccy", "Safe", "TR flags"}, rows)
	if len(errs) > 0 {
		s += "\nErrors:\n- " + strings.Join(errs, "\n- ") + "\n"
	}
	_, err := fmt.Fprint(e.out, s)
	return err
}

// isREIT reads the REIT flag off a stored dossier.
func (e *env) isREIT(ticker string) bool {
	var d eulerpool.Dossier
	ok, _ := e.readYAML(&d, "stocks", ticker, "financial_data.yaml")
	return ok && rules.IsREIT(d.Identity.Sector, d.Identity.Industry)
}

// fxTable is the home currency (the country pack's) per unit of each currency, from data/macro.yaml.
type fxTable map[string]float64

func (e *env) fxTable() fxTable {
	var m eulerpool.Macro
	home := pack.Home()
	t := fxTable{home: 1}
	if ok, _ := e.readYAML(&m, "macro.yaml"); !ok {
		return t
	}
	for _, r := range m.FX {
		if base, quote, ok := strings.Cut(r.Pair, "/"); ok && strings.EqualFold(quote, home) && r.Rate > 0 {
			t[strings.ToUpper(base)] = r.Rate
		}
	}
	return t
}

// toUSD converts an amount in ccy into USD through the home currency.
func (t fxTable) toUSD(v float64, ccy string) (float64, bool) {
	if ccy == "USD" {
		return v, true
	}
	from, ok1 := t[ccy]
	usd, ok2 := t["USD"]
	if !ok1 || !ok2 || usd == 0 {
		return 0, false
	}
	return v * from / usd, true
}

// priceToFairValue is the price over the human view's fair value, the fair value first converted into
// the price's currency through data/macro.yaml's rates (a CAD price and a USD fair value; a USD
// price and a GBX fair value). Without a rate it says so instead.
func (t fxTable) priceToFairValue(price float64, priceCcy string, fv float64, fvCcy string) (*float64, string, bool) {
	pc, fc := normCcy(priceCcy), normCcy(fvCcy)
	fv *= minorUnit(fvCcy)
	if fc == "" || pc == "" || fc == pc {
		return rounded(price/fv, 2), "", true
	}
	a, ok1 := t[fc]
	b, ok2 := t[pc]
	if !ok1 || !ok2 || b == 0 {
		h := pack.Home()
		return nil, fmt.Sprintf("pb derive: price in %s, fair value in %s, and no %s/%s or %s/%s rate in data/macro.yaml: no P/FV (run pb ep macro)", pc, fvCcy, fc, h, pc, h), false
	}
	conv := fv * a / b
	h := pack.Home()
	note := fmt.Sprintf("fair value %s %s = %s %s at %s/%s %s and %s/%s %s (data/macro.yaml)", strconv.FormatFloat(fv, 'f', -1, 64), fc,
		strconv.FormatFloat(round2v(conv), 'f', -1, 64), pc, fc, h, strconv.FormatFloat(a, 'f', -1, 64), pc, h, strconv.FormatFloat(b, 'f', -1, 64))
	return rounded(price/conv, 2), note, true
}

// fromHome converts an amount in the home currency into ccy (for the fee check).
func (t fxTable) fromHome(v float64, ccy string) (float64, bool) {
	r, ok := t[strings.ToUpper(ccy)]
	if !ok || r == 0 {
		return 0, false
	}
	return v / r, true
}

// trFlagsText names the total-return flags that are raised or need a look ("–" when none).
func trFlagsText(cs []rules.Check) string {
	var out []string
	for _, c := range cs {
		switch c.Status {
		case rules.Flagged:
			out = append(out, c.Name)
		case rules.NeedsLook:
			out = append(out, c.Name+"?")
		}
	}
	if len(out) == 0 {
		return "–"
	}
	return strings.Join(out, " ")
}

// NameYoCPath is one name's net yield on cost by year, on its projected dividend growth.
type NameYoCPath struct {
	NetYieldPct float64   `yaml:"net_yield_pct" json:"net_yield_pct"`
	GrowthPct   float64   `yaml:"growth_pct" json:"growth_pct"`
	GrowthBasis string    `yaml:"growth_basis" json:"growth_basis"`
	YearsPct    []float64 `yaml:"years_pct" json:"years_pct"` // years 1..10: net yield × (1+g)^(n-1)
}

// nameYoCPath projects the net yield on cost of one share bought today; nil without a net yield
// or any dividend growth figure.
func nameYoCPath(netYield *float64, growth map[string]*float64) *NameYoCPath {
	if netYield == nil || *netYield <= 0 {
		return nil
	}
	g, basis := rules.ProjectedDividendGrowth(growth["10y"], growth["5y"], growth["3y"])
	if g == nil {
		return nil
	}
	p := &NameYoCPath{NetYieldPct: *netYield, GrowthPct: *g, GrowthBasis: basis}
	for n := 0; n < 10; n++ {
		p.YearsPct = append(p.YearsPct, math.Round(*netYield*math.Pow(1+*g/100, float64(n))*100)/100)
	}
	return p
}
