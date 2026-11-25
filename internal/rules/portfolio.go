package rules

// portfolio.go - the portfolio check against the investor's strategy: split, caps, income by
// currency, fees, the 10-year yield-on-cost path and the excluded names found inside the funds.

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

// Line kinds in an allocation.
const (
	KindStock   = "stock"
	KindETFDist = "etf_dist"
	KindETFAcc  = "etf_acc"
)

// DefaultYears is the horizon a mix with none is projected over. Every limit the check judges by is
// the investor's strategy (pack.Strategy); an unset one skips its rule.
const DefaultYears = 10

// InflationPct is the country pack's long-run inflation, the default for a mix that states none:
// the dividend growth after year 10 must beat the mix's inflation, and real values are deflated by it.
func InflationPct() float64 { return pack.Active().Country.InflationPct }

// Allocation is allocation.yaml: the plan Jack proposes and pb portfolio check reads.
// Amounts are in Currency (the country pack's currency by default); weights and rates are percent.
type Allocation struct {
	AsOf           string  `yaml:"as_of,omitempty" json:"as_of,omitempty"`
	Currency       string  `yaml:"currency,omitempty" json:"currency,omitempty"`
	StartAmount    float64 `yaml:"start_amount,omitempty" json:"start_amount,omitempty"`
	Monthly        float64 `yaml:"monthly,omitempty" json:"monthly,omitempty"`
	Years          int     `yaml:"years,omitempty" json:"years,omitempty"`
	StartYear      int     `yaml:"start_year,omitempty" json:"start_year,omitempty"`
	PriceGrowthPct float64 `yaml:"price_growth_pct,omitempty" json:"price_growth_pct,omitempty"`
	InflationPct   float64 `yaml:"inflation_pct,omitempty" json:"inflation_pct,omitempty"`
	Lines          []Line  `yaml:"lines" json:"lines"`
	// Switch states the switch-to-income inputs (switch.go); absent fields take the strategy's.
	Switch *SwitchParams `yaml:"switch,omitempty" json:"switch,omitempty"`
}

// Line is one holding. Only Ticker, Kind and WeightPct are required; pb fills the
// rest from data/ (derived.yaml, broker.yaml, watchlist.yaml) where it is absent.
type Line struct {
	Ticker string `yaml:"ticker" json:"ticker"`
	Name   string `yaml:"name,omitempty" json:"name,omitempty"`
	Kind   string `yaml:"kind" json:"kind"`
	Bucket string `yaml:"bucket,omitempty" json:"bucket,omitempty"`
	// Group is the strategy group from derived.yaml (GOAT, Yielder, …, Growth); the switch sells Growth.
	Group             string   `yaml:"group,omitempty" json:"group,omitempty"`
	WeightPct         float64  `yaml:"weight_pct" json:"weight_pct"`
	YieldPct          *float64 `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	DividendGrowthPct *float64 `yaml:"dividend_growth_pct,omitempty" json:"dividend_growth_pct,omitempty"`
	PriceGrowthPct    *float64 `yaml:"price_growth_pct,omitempty" json:"price_growth_pct,omitempty"`
	Country           string   `yaml:"country,omitempty" json:"country,omitempty"`
	ISIN              string   `yaml:"isin,omitempty" json:"isin,omitempty"`
	REIT              bool     `yaml:"reit,omitempty" json:"reit,omitempty"`
	WithholdingPct    *float64 `yaml:"withholding_pct,omitempty" json:"withholding_pct,omitempty"`
	DividendCurrency  string   `yaml:"dividend_currency,omitempty" json:"dividend_currency,omitempty"`
	TradingCurrency   string   `yaml:"trading_currency,omitempty" json:"trading_currency,omitempty"`
	Market            string   `yaml:"market,omitempty" json:"market,omitempty"`
	Frequency         string   `yaml:"frequency,omitempty" json:"frequency,omitempty"`
	Moonshot          bool     `yaml:"moonshot,omitempty" json:"moonshot,omitempty"`
	// SwitchRole overrides what the switch to income does with the line: "sell" or "keep" (switch.go).
	SwitchRole string `yaml:"switch,omitempty" json:"switch,omitempty"`
	// ExcludedPct is the share of a fund's published top holdings that the strategy's
	// excluded_tickers name (a look-through, reported and not capped).
	ExcludedPct *float64 `yaml:"excluded_pct,omitempty" json:"excluded_pct,omitempty"`
	// GrowthBasis says where a filled dividend growth came from (and whether it was capped).
	GrowthBasis string `yaml:"growth_basis,omitempty" json:"growth_basis,omitempty"`
	// CurrencyBasis says when the dividend currency was assumed rather than read.
	CurrencyBasis string `yaml:"currency_basis,omitempty" json:"currency_basis,omitempty"`
}

// IsETF reports a fund line.
func (l Line) IsETF() bool { return l.Kind == KindETFDist || l.Kind == KindETFAcc }

// Rule is one portfolio rule's outcome.
type Rule struct {
	Name   string `yaml:"name" json:"name"`
	Status string `yaml:"status" json:"status"`
	Detail string `yaml:"detail" json:"detail"`
}

// LineReport is one line after the check: its income share and fee.
type LineReport struct {
	Ticker         string   `yaml:"ticker" json:"ticker"`
	Kind           string   `yaml:"kind" json:"kind"`
	WeightPct      float64  `yaml:"weight_pct" json:"weight_pct"`
	YieldPct       *float64 `yaml:"yield_pct,omitempty" json:"yield_pct,omitempty"`
	WithholdingPct float64  `yaml:"withholding_pct" json:"withholding_pct"`
	NetYieldPct    *float64 `yaml:"net_yield_pct,omitempty" json:"net_yield_pct,omitempty"`
	IncomeSharePct *float64 `yaml:"income_share_pct,omitempty" json:"income_share_pct,omitempty"`
	Currency       string   `yaml:"dividend_currency,omitempty" json:"dividend_currency,omitempty"`
	// Moonshot echoes the line's flag: its income cap is the moonshot one.
	Moonshot   bool     `yaml:"moonshot,omitempty" json:"moonshot,omitempty"`
	MonthlyBuy float64  `yaml:"monthly_buy" json:"monthly_buy"`
	FeePct     *float64 `yaml:"fee_pct,omitempty" json:"fee_pct,omitempty"`
	Notes      []string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// YearRow is one year of the yield-on-cost path (yoc.go). Yields on cost are the next twelve
// months' net income at the year-end holdings over the money they are measured on.
type YearRow struct {
	Year          int     `yaml:"year" json:"year"`
	Contributed   float64 `yaml:"contributed" json:"contributed"`
	Value         float64 `yaml:"value" json:"value"`
	RealValue     float64 `yaml:"real_value" json:"real_value"`
	NetIncome     float64 `yaml:"net_income" json:"net_income"`
	MonthlyIncome float64 `yaml:"monthly_income" json:"monthly_income"`
	ForwardNet    float64 `yaml:"forward_net" json:"forward_net"`
	YoCPct        float64 `yaml:"yoc_on_contributed_pct" json:"yoc_on_contributed_pct"`
	CohortYoCPct  float64 `yaml:"start_money_yoc_pct" json:"start_money_yoc_pct"`
	PlainYoCPct   float64 `yaml:"plain_yoc_pct" json:"plain_yoc_pct"`
	Reinvested    float64 `yaml:"reinvested" json:"reinvested"`
}

// Report is pb portfolio check's answer.
type Report struct {
	Currency            string             `yaml:"currency" json:"currency"`
	ETFPct              float64            `yaml:"etf_pct" json:"etf_pct"`
	StockPct            float64            `yaml:"stock_pct" json:"stock_pct"`
	StartYieldPct       *float64           `yaml:"start_yield_pct,omitempty" json:"start_yield_pct,omitempty"`
	StartNetYieldPct    *float64           `yaml:"start_net_yield_pct,omitempty" json:"start_net_yield_pct,omitempty"`
	PriceGrowthPct      float64            `yaml:"price_growth_pct" json:"price_growth_pct"`
	DividendGrowthPct   *float64           `yaml:"dividend_growth_pct,omitempty" json:"dividend_growth_pct,omitempty"`
	IncomeByCurrency    map[string]float64 `yaml:"income_by_currency_pct" json:"income_by_currency_pct"`
	ETFCurrencies       []string           `yaml:"etf_currencies" json:"etf_currencies"`
	MonthlyFeePct       *float64           `yaml:"monthly_fee_pct,omitempty" json:"monthly_fee_pct,omitempty"`
	ExcludedInFundsPct  *float64           `yaml:"excluded_in_funds_pct,omitempty" json:"excluded_in_funds_pct,omitempty"`
	ExcludedInFundsNote string             `yaml:"excluded_in_funds_note,omitempty" json:"excluded_in_funds_note,omitempty"`
	Rules               []Rule             `yaml:"rules" json:"rules"`
	Lines               []LineReport       `yaml:"lines" json:"lines"`
	Path                []YearRow          `yaml:"yoc_path" json:"yoc_path"`
	Reinvest            []string           `yaml:"reinvestment,omitempty" json:"reinvestment,omitempty"`
	Switch              *SwitchReport      `yaml:"switch,omitempty" json:"switch,omitempty"`
	Notes               []string           `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// FX converts an amount in the allocation's currency into a market currency;
// ok is false where no rate is known.
type FX func(amount float64, to string) (float64, bool)

// CheckPortfolio runs every portfolio rule over a filled allocation.
func CheckPortfolio(a Allocation, fx FX) Report {
	a = withDefaults(a)
	r := Report{Currency: a.Currency, IncomeByCurrency: map[string]float64{}, PriceGrowthPct: a.PriceGrowthPct}
	var total float64
	for _, l := range a.Lines {
		total += l.WeightPct
	}
	if total <= 0 {
		r.Rules = append(r.Rules, Rule{"weights", Fail, "no line has a weight"})
		return r
	}
	if math.Abs(total-100) > 0.5 {
		r.Notes = append(r.Notes, fmt.Sprintf("weights add to %.2f%%; they are normalised to 100%%", total))
	}

	// Per line: normalised weight, gross and net income per 100 of portfolio.
	var gross, net, growthW, growthNet, priceW float64
	yieldKnown := true
	var noGrowth, noWHT []string
	type inc struct{ net float64 }
	incomes := make([]inc, len(a.Lines))
	etfCcy := map[string]bool{}
	for i, l := range a.Lines {
		w := l.WeightPct / total * 100
		lr := LineReport{Ticker: l.Ticker, Kind: l.Kind, WeightPct: round(w, 2), YieldPct: l.YieldPct, Currency: l.DividendCurrency, Moonshot: l.Moonshot}
		if strings.Contains(l.GrowthBasis, "capped") {
			lr.Notes = append(lr.Notes, "dividend growth "+l.GrowthBasis)
		}
		if l.CurrencyBasis != "" {
			lr.Notes = append(lr.Notes, l.CurrencyBasis)
		}
		if l.IsETF() && l.DividendCurrency != "" && l.TradingCurrency != "" && !strings.EqualFold(l.DividendCurrency, l.TradingCurrency) {
			lr.Notes = append(lr.Notes, "pays "+strings.ToUpper(l.DividendCurrency)+", trades in "+strings.ToUpper(l.TradingCurrency)+": its income is reinvested as "+strings.ToUpper(l.DividendCurrency)+", and it is the reinvestment line for "+strings.ToUpper(l.TradingCurrency)+" income")
		}
		if l.IsETF() {
			r.ETFPct += w
			if c := strings.ToUpper(l.TradingCurrency); c != "" {
				etfCcy[c] = true
			} else {
				lr.Notes = append(lr.Notes, "trading currency unknown: cannot match income currencies")
			}
		} else {
			r.StockPct += w
		}
		y := lineYield(l)
		if y == nil {
			yieldKnown = false
			lr.Notes = append(lr.Notes, "yield unknown: needs a look")
		}
		wh, whKnown := lineWithholding(l)
		lr.WithholdingPct = wh
		if !whKnown && y != nil && *y > 0 {
			noWHT = append(noWHT, l.Ticker)
			lr.Notes = append(lr.Notes, "withholding unknown: no rate in the packs for "+cmpStr(l.Country, FundDomicile(l.ISIN), "its country")+", projected gross (an upper bound); state withholding_pct or add the rate to the country pack")
		}
		if y != nil {
			g := w * *y / 100
			n := g * (1 - wh/100)
			gross, net = gross+g, net+n
			incomes[i] = inc{n}
			lr.NetYieldPct = round2(*y * (1 - wh/100))
			if c := strings.ToUpper(l.DividendCurrency); n > 0 {
				if c == "" {
					c = "unknown"
				}
				r.IncomeByCurrency[c] += n
			}
			if l.DividendGrowthPct != nil {
				growthW += n * *l.DividendGrowthPct
				growthNet += n
			} else if n > 0 {
				noGrowth = append(noGrowth, l.Ticker)
				lr.Notes = append(lr.Notes, "dividend growth unknown (no source): projected at 0%; state dividend_growth_pct in the mix")
			}
		}
		pg := a.PriceGrowthPct
		if l.PriceGrowthPct != nil {
			pg = *l.PriceGrowthPct
		}
		priceW += w * pg
		r.Lines = append(r.Lines, lr)
	}
	r.ETFPct, r.StockPct = round(r.ETFPct, 2), round(r.StockPct, 2)
	r.PriceGrowthPct = round(priceW/100, 2)
	for c := range etfCcy {
		r.ETFCurrencies = append(r.ETFCurrencies, c)
	}
	slices.Sort(r.ETFCurrencies)
	if yieldKnown {
		r.StartYieldPct, r.StartNetYieldPct = round2(gross), round2(net)
	} else if gross > 0 {
		r.Notes = append(r.Notes, "some yields are unknown, so the starting yield and income shares are partial")
		r.StartYieldPct, r.StartNetYieldPct = round2(gross), round2(net)
	}
	if growthNet > 0 {
		r.DividendGrowthPct = round2(growthW / growthNet)
	}
	if net > 0 {
		for i := range r.Lines {
			if a.Lines[i].Kind == KindETFAcc {
				continue
			}
			r.Lines[i].IncomeSharePct = round2(incomes[i].net / net * 100)
		}
		for c, v := range r.IncomeByCurrency {
			r.IncomeByCurrency[c] = round(v/net*100, 2)
		}
	}

	r.split()
	r.caps(a)
	r.currencies()
	r.fees(a, fx)
	r.excludedInFunds(a)
	r.path(a)
	r.withholding(noWHT)
	r.targets(a, noGrowth, noWHT)
	for _, f := range portfolioChecks {
		f(a, &r)
	}
	return r
}

// withholding reports the lines whose withholding no pack states.
func (r *Report) withholding(unknown []string) {
	if len(unknown) == 0 {
		r.add("withholding", Pass, "every paying line has a withholding rate from the packs or the mix")
		return
	}
	r.add("withholding", NeedsLook, "no withholding rate for "+strings.Join(unknown, ", ")+": their income is projected gross, so net yield, income shares and yield on cost are upper bounds")
}

func cmpStr(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

func withDefaults(a Allocation) Allocation {
	c := pack.Active().Country
	st := pack.Active().Strategy
	if a.Currency == "" {
		a.Currency = strings.ToUpper(c.Currency)
	}
	if a.Years <= 0 {
		a.Years = DefaultYears
	}
	if a.PriceGrowthPct == 0 && st.PriceGrowthPct != nil {
		a.PriceGrowthPct = *st.PriceGrowthPct
	}
	if a.InflationPct == 0 {
		a.InflationPct = c.InflationPct
	}
	if a.StartAmount == 0 && a.Monthly == 0 {
		if st.StartAmount != nil {
			a.StartAmount = *st.StartAmount
		}
		if st.Monthly != nil {
			a.Monthly = *st.Monthly
		}
	}
	if a.StartYear == 0 {
		a.StartYear = time.Now().Year() + 1
	}
	// Every line carries its price growth, so the page projects on the same rates as pb.
	a.Lines = slices.Clone(a.Lines)
	for i := range a.Lines {
		if a.Lines[i].PriceGrowthPct == nil {
			pg := a.PriceGrowthPct
			a.Lines[i].PriceGrowthPct = &pg
		}
	}
	return a
}

// lineYield is a line's distribution yield; an accumulating fund yields nothing.
func lineYield(l Line) *float64 {
	if l.Kind == KindETFAcc {
		z := 0.0
		return &z
	}
	return l.YieldPct
}

// lineWithholding is the rate a line's dividend loses at source at the active broker, and whether
// any pack (or the mix) states it. A fund's rate is by its domicile (FundWithholding). An unknown
// rate is 0 in the projection, and the check says so (rule withholding).
func lineWithholding(l Line) (float64, bool) {
	if l.WithholdingPct != nil {
		return *l.WithholdingPct, true
	}
	if l.IsETF() {
		if w := FundWithholding(l.ISIN); w != nil {
			return w.RatePct, true
		}
		return 0, false
	}
	if w := WithholdingFor(l.Country, l.ISIN, l.REIT); w != nil {
		return w.RatePct, true
	}
	return 0, false
}

func (r *Report) add(name, status, detail string) {
	r.Rules = append(r.Rules, Rule{name, status, detail})
}

func (r *Report) split() {
	s := pack.Active().Strategy
	if s.ETFMinPct == nil || s.ETFMaxPct == nil {
		r.add("etf_share", Skipped, fmt.Sprintf("ETFs %.1f%%, stocks %.1f%%; the ETF share band is %s", r.ETFPct, r.StockPct, pack.UnsetNote))
		return
	}
	st := Pass
	if r.ETFPct < *s.ETFMinPct || r.ETFPct > *s.ETFMaxPct {
		st = Fail
	}
	r.add("etf_share", st, fmt.Sprintf("ETFs %.1f%%, stocks %.1f%% (ETFs must be %g–%g%%)", r.ETFPct, r.StockPct, *s.ETFMinPct, *s.ETFMaxPct))
}

func (r *Report) caps(a Allocation) {
	st := pack.Active().Strategy
	var over, income, moon []string
	for i, l := range a.Lines {
		lr := r.Lines[i]
		if !l.IsETF() && st.WeightCapPct != nil && lr.WeightPct > *st.WeightCapPct {
			over = append(over, fmt.Sprintf("%s %.1f%%", l.Ticker, lr.WeightPct))
		}
		if lr.IncomeSharePct == nil {
			continue
		}
		s := *lr.IncomeSharePct
		if l.Moonshot && st.MoonshotIncomeCapPct != nil && s > *st.MoonshotIncomeCapPct {
			moon = append(moon, fmt.Sprintf("%s %.1f%%", l.Ticker, s))
		}
		limit := st.IncomeCapPct
		if l.IsETF() {
			limit = st.ETFIncomeCapPct
		}
		if !l.Moonshot && limit != nil && s > *limit {
			income = append(income, fmt.Sprintf("%s %.1f%% (cap %g%%)", l.Ticker, s, *limit))
		}
	}
	if st.WeightCapPct == nil {
		r.add("weight_cap", Skipped, "the stock weight cap is "+pack.UnsetNote)
	} else {
		r.add("weight_cap", statusOf(over), fmt.Sprintf("stock weight <= %g%% (ETFs exempt: look-through) ", *st.WeightCapPct)+listOr(over))
	}
	if st.IncomeCapPct == nil && st.ETFIncomeCapPct == nil {
		r.add("income_cap", Skipped, "the income caps are "+pack.UnsetNote)
	} else {
		r.add("income_cap", statusOf(income), "income per stock <= "+pctOr(st.IncomeCapPct)+", per ETF <= "+pctOr(st.ETFIncomeCapPct)+" "+listOr(income))
	}
	if st.MoonshotIncomeCapPct == nil {
		r.add("moonshot_income_cap", Skipped, "the moonshot income cap is "+pack.UnsetNote)
	} else {
		r.add("moonshot_income_cap", statusOf(moon), fmt.Sprintf("income per moonshot <= %g%% ", *st.MoonshotIncomeCapPct)+listOr(moon))
	}
}

// pctOr prints a limit, or says it is unset.
func pctOr(p *float64) string {
	if p == nil {
		return "not set in " + pack.StrategyFile
	}
	return fmt.Sprintf("%g%%", *p)
}

func (r *Report) currencies() {
	caps := pack.Active().Strategy.CurrencyIncomeCapPct
	for _, c := range slices.Sorted(maps.Keys(caps)) {
		share, limit := r.IncomeByCurrency[strings.ToUpper(c)], caps[c]
		st := Pass
		if share > limit {
			st = Fail
		}
		r.add(strings.ToLower(c)+"_income", st, fmt.Sprintf("%s income %.1f%% (<= %g%%)", strings.ToUpper(c), share, limit))
	}
	st := Pass
	var missing, exempt []string
	for c, v := range r.IncomeByCurrency {
		if v <= 0 {
			continue
		}
		if StockReinvestCurrency(c) {
			exempt = append(exempt, c)
			continue
		}
		if !slices.Contains(r.ETFCurrencies, c) {
			missing = append(missing, c)
		}
	}
	slices.Sort(missing)
	st = Pass
	if len(missing) > 0 {
		st = Fail
		if slices.Contains(missing, "unknown") {
			st = NeedsLook
		}
	}
	slices.Sort(exempt)
	ex := ""
	if len(exempt) > 0 {
		ex = " (" + strings.Join(exempt, ", ") + " reinvested into its own stocks: stock_reinvest_currencies)"
	}
	r.add("currency_match", st, strings.TrimSpace("every income currency has an ETF trading in it"+ex+" "+listOr(missing)))
}

func (r *Report) fees(a Allocation, fx FX) {
	if a.Monthly <= 0 {
		r.add("fees", Skipped, "no monthly contribution")
		return
	}
	var cost float64
	known := true
	p := pack.Active()
	for i, l := range a.Lines {
		buy := a.Monthly * r.Lines[i].WeightPct / 100
		r.Lines[i].MonthlyBuy = round(buy, 0)
		if buy <= 0 {
			continue
		}
		key := l.Market
		if key == "" {
			key = DefaultMarket(l)
		}
		mccy, ok := p.Broker.Currency(key)
		if !ok {
			known = false
			r.Lines[i].Notes = append(r.Lines[i].Notes, "fee market "+key+" unknown: needs a look")
			continue
		}
		amt, ok := fx(buy, mccy)
		if !ok {
			known = false
			r.Lines[i].Notes = append(r.Lines[i].Notes, "no FX rate to "+mccy+": fee needs a look")
			continue
		}
		q := p.Broker.Quote(key, amt)
		if !q.Offered {
			known = false
			r.Lines[i].Notes = append(r.Lines[i].Notes, p.Broker.Name+": "+q.Why)
			continue
		}
		pct := q.Total / amt * 100
		// The home market's transaction tax (the country pack's levy) is charged on top, judged on
		// the buy in the home currency; a venue whose broker price already carries a tax is not
		// taxed twice.
		if m, ok := p.Broker.Markets[key]; !ok || m.TaxPct == 0 {
			if home, ok := fx(buy, pack.Home()); ok && home > 0 {
				for _, x := range p.Levy(key, home) {
					pct += x.Amount / home * 100
				}
			}
		}
		r.Lines[i].FeePct = round2(pct)
		cost += buy * pct / 100
	}
	avg := cost / a.Monthly * 100
	r.MonthlyFeePct = round2(avg)
	limit := p.Strategy.FeeMaxPct
	st := Pass
	switch {
	case limit == nil:
		st = Skipped
	case avg > *limit:
		st = Fail
	case !known:
		st = NeedsLook
	}
	r.add("fees", st, fmt.Sprintf("monthly trading cost %.3f%% of %s %.0f at %s, transaction taxes included, FX excluded (limit %s)", avg, a.Currency, a.Monthly, p.Broker.Name, pctOr(limit)))
}

// DefaultMarket is the fee market a line is bought on when the allocation names none: the broker
// pack's fund market, the US market for a USD line, else the market of the line's country.
func DefaultMarket(l Line) string {
	b := pack.Active().Broker
	if l.IsETF() {
		return b.DefaultETFMarket
	}
	us := b.MarketByCountry["US"]
	if strings.EqualFold(l.TradingCurrency, "USD") && us != "" {
		return us
	}
	if m, ok := b.MarketByCountry[strings.ToUpper(l.Country)]; ok {
		return m
	}
	return b.DefaultMarket
}

// excludedInFunds reports how much of the portfolio sits, through the funds' published top
// holdings, in names the strategy excludes (excluded_tickers): reported, not capped.
func (r *Report) excludedInFunds(a Allocation) {
	if len(pack.Active().Strategy.ExcludedTickers) == 0 {
		r.add("excluded_in_funds", Skipped, "no excluded_tickers in "+pack.StrategyFile+" to look for inside the funds")
		return
	}
	var pct float64
	var unknown []string
	for i, l := range a.Lines {
		if !l.IsETF() {
			continue
		}
		if l.ExcludedPct == nil {
			unknown = append(unknown, l.Ticker)
			continue
		}
		pct += r.Lines[i].WeightPct * *l.ExcludedPct / 100
	}
	r.ExcludedInFundsPct = round2(pct)
	r.ExcludedInFundsNote = "look-through from the funds' published top holdings only"
	if len(unknown) > 0 {
		r.ExcludedInFundsNote += "; holdings unknown for " + strings.Join(unknown, ", ")
	}
	r.add("excluded_in_funds", Pass, fmt.Sprintf("names your strategy excludes, inside the funds: %.2f%% of the portfolio (reported, not capped)", pct))
}

// path runs the 10-year path twice (yoc.go): with every contribution, and on the start money alone,
// so monthly contributions never inflate the yield on the start money.
func (r *Report) path(a Allocation) {
	lines := make([]YoCLine, len(a.Lines))
	for i, l := range a.Lines {
		y := lineYield(l)
		x := YoCLine{Weight: r.Lines[i].WeightPct, WithholdingPct: r.Lines[i].WithholdingPct, PriceGrowthPct: a.PriceGrowthPct, Fund: l.IsETF(),
			Acc: l.Kind == KindETFAcc, Currency: payCurrency(l), BuyCurrency: buyCurrency(l)}
		if y != nil {
			x.YieldPct = *y
		}
		if l.DividendGrowthPct != nil {
			x.GrowthPct = *l.DividendGrowthPct
		}
		if l.PriceGrowthPct != nil {
			x.PriceGrowthPct = *l.PriceGrowthPct
		}
		lines[i] = x
	}
	r.reinvestNotes(a, lines)
	var sp SwitchParams
	if a.Switch != nil {
		sp = *a.Switch
	}
	r.Switch = ProjectSwitch(a, lines, sp)
	full := ProjectYoC(lines, a.StartAmount, a.Monthly, a.Years, a.InflationPct)
	cohort := ProjectYoC(lines, a.StartAmount, 0, a.Years, a.InflationPct)
	for i, y := range full {
		row := YearRow{Year: a.StartYear + y.N - 1, Contributed: round(y.Contributed, 0), Value: round(y.Value, 0),
			RealValue: round(y.RealValue, 0), NetIncome: round(y.Income, 0), MonthlyIncome: round(y.Income/12, 0),
			ForwardNet: round(y.Forward, 0), Reinvested: round(y.Income, 0), PlainYoCPct: round(PlainYoCPct(lines, y.N), 2)}
		if y.Contributed > 0 {
			row.YoCPct = round(y.Forward/y.Contributed*100, 2)
		}
		if i < len(cohort) && a.StartAmount > 0 {
			row.CohortYoCPct = round(cohort[i].Forward/a.StartAmount*100, 2)
		}
		r.Path = append(r.Path, row)
	}
	for _, l := range a.Lines {
		if y := lineYield(l); l.Kind != KindETFAcc && (y == nil || *y <= 0) {
			r.Notes = append(r.Notes, l.Ticker+": no yield, so it is projected as paying nothing")
		}
	}
}

// payCurrency is the currency a line's dividend arrives in: the stated dividend currency, else the
// one it trades in. A line with no income has none.
func payCurrency(l Line) string {
	if y := lineYield(l); y == nil || *y <= 0 {
		return ""
	}
	return strings.ToUpper(firstNonEmptyStr(l.DividendCurrency, l.TradingCurrency))
}

// buyCurrency is the currency a line is bought with: its trading currency, else (a stock) the
// currency it pays in.
func buyCurrency(l Line) string {
	if l.IsETF() {
		return strings.ToUpper(l.TradingCurrency)
	}
	return strings.ToUpper(firstNonEmptyStr(l.TradingCurrency, l.DividendCurrency))
}

func firstNonEmptyStr(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// reinvestNotes says, per income currency, where its dividends are reinvested and whether FX is paid.
func (r *Report) reinvestNotes(a Allocation, lines []YoCLine) {
	seen := map[string]bool{}
	for _, l := range lines {
		c := strings.ToUpper(l.Currency)
		if c == "" || seen[c] || l.YieldPct <= 0 {
			continue
		}
		seen[c] = true
		var to []string
		fx := false
		for _, t := range ReinvestTargets(lines, c) {
			to = append(to, a.Lines[t.Line].Ticker)
			fx = fx || t.FXCostPct > 0
		}
		note := c + " dividends are reinvested into " + strings.Join(to, ", ")
		if fx {
			note += fmt.Sprintf(" after FX (%.2f%% cost): no line trades in %s", FXCostPct(), c)
		}
		r.Reinvest = append(r.Reinvest, note)
	}
	slices.Sort(r.Reinvest)
}

func (r *Report) targets(a Allocation, noGrowth, noWHT []string) {
	st := pack.Active().Strategy
	if n := len(r.Path); n > 0 {
		target := st.YoCYear10Pct
		y10 := r.Path[min(DefaultYears, n)-1]
		// The rule is about year 10, whatever horizon the mix runs: a 20-year path is read at its 10th
		// row. With no start money it is judged on all the money paid in.
		onStart := a.StartAmount > 0
		got, basis := y10.CohortYoCPct, "on the start money"
		if !onStart {
			got, basis = y10.YoCPct, "on all money paid in (no start amount)"
		}
		status := Pass
		switch {
		case target == nil:
			status = Skipped
		case n < DefaultYears:
			status = Skipped
		case got < *target:
			status = Fail
		case len(noGrowth)+len(noWHT) > 0:
			status = NeedsLook
		}
		detail := fmt.Sprintf("year %d (year %d of the path) net yield %s %.2f%% (target %s); on all money paid in %.2f%%",
			y10.Year, min(DefaultYears, n), basis, got, pctOr(target), y10.YoCPct)
		if n < DefaultYears {
			detail += fmt.Sprintf("; the path runs only %d years, so year 10 is not judged", n)
		}
		if len(noGrowth) > 0 {
			detail += "; projected at 0% dividend growth for " + strings.Join(noGrowth, ", ")
		}
		if len(noWHT) > 0 {
			detail += "; projected gross for " + strings.Join(noWHT, ", ")
		}
		r.add("yoc_year_10", status, detail)
	}
	if pg := st.PriceGrowthPct; pg == nil {
		r.add("price_growth", Skipped, fmt.Sprintf("weighted price growth assumption %.2f%%; the price-growth assumption and target are %s, so a line with none is projected at 0%%", r.PriceGrowthPct, pack.UnsetNote))
	} else {
		tol := 0.0
		if st.PriceGrowthTolerancePct != nil {
			tol = *st.PriceGrowthTolerancePct
		}
		status := Pass
		if r.PriceGrowthPct < *pg-tol {
			status = Fail
		}
		r.add("price_growth", status, fmt.Sprintf("weighted price growth assumption %.2f%% (target %g%%, at most %g below)", r.PriceGrowthPct, *pg, tol))
	}
	infl := a.InflationPct
	switch {
	case r.DividendGrowthPct == nil:
		r.add("growth_after_10", NeedsLook, "dividend growth unknown")
	case *r.DividendGrowthPct <= infl:
		r.add("growth_after_10", Fail, fmt.Sprintf("income-weighted dividend growth %.2f%% (> %g%% inflation)%s", *r.DividendGrowthPct, infl, unknownGrowth(noGrowth)))
	case len(noGrowth) > 0:
		r.add("growth_after_10", NeedsLook, fmt.Sprintf("income-weighted dividend growth %.2f%% (> %g%% inflation) over the lines with a growth rate%s", *r.DividendGrowthPct, infl, unknownGrowth(noGrowth)))
	default:
		r.add("growth_after_10", Pass, fmt.Sprintf("income-weighted dividend growth %.2f%% (> %g%% inflation)", *r.DividendGrowthPct, infl))
	}
}

func unknownGrowth(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return "; no growth rate for " + strings.Join(xs, ", ")
}

func statusOf(breaches []string) string {
	if len(breaches) > 0 {
		return Fail
	}
	return Pass
}

func listOr(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return "— breached: " + strings.Join(xs, ", ")
}

func round(v float64, dp int) float64 {
	p := math.Pow(10, float64(dp))
	return math.Round(v*p) / p
}
