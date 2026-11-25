package pack

// strategy.go - the investor's own strategy: data/profile/strategy.yaml, written by /profile from
// the questionnaire's answers. Nothing here has a default: an unset limit means the rule that needs
// it is skipped and says so, never quietly filled with someone else's number.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v4"
)

// StrategyFile is where the strategy lives under the data folder.
const StrategyFile = "profile/strategy.yaml"

// Strategy is every number the rules judge by. Pointers are unset when nil.
type Strategy struct {
	// The money (in the home currency): what a mix with no amounts is projected on, the net
	// monthly income the switch searches for, and a higher monthly contribution shown beside it.
	StartAmount   *float64 `yaml:"start_amount,omitempty" json:"start_amount,omitempty"`
	Monthly       *float64 `yaml:"monthly,omitempty" json:"monthly,omitempty"`
	TargetMonthly *float64 `yaml:"target_monthly,omitempty" json:"target_monthly,omitempty"`
	HigherMonthly *float64 `yaml:"higher_monthly,omitempty" json:"higher_monthly,omitempty"`
	// Dividend groups.
	GroupYieldPct     *float64 `yaml:"group_yield_pct,omitempty" json:"group_yield_pct,omitempty"`         // Yielder and GOAT: yield at least this
	GOATGrowthPct     *float64 `yaml:"goat_growth_pct,omitempty" json:"goat_growth_pct,omitempty"`         // GOAT: and dividend growth at least this
	GrowerGrowthPct   *float64 `yaml:"grower_growth_pct,omitempty" json:"grower_growth_pct,omitempty"`     // Grower: dividend growth at least this
	StableGrowerYears *int     `yaml:"stable_grower_years,omitempty" json:"stable_grower_years,omitempty"` // years of increases
	StablePayerYears  *int     `yaml:"stable_payer_years,omitempty" json:"stable_payer_years,omitempty"`   // years with no cut
	LowYieldPct       *float64 `yaml:"low_yield_pct,omitempty" json:"low_yield_pct,omitempty"`             // below it a name with no group is Growth
	// LargeCapOnlyGroups are the groups a mid or small cap cannot join (e.g. [Grower, Stable Grower]).
	LargeCapOnlyGroups []string `yaml:"large_cap_only_groups,omitempty" json:"large_cap_only_groups,omitempty"`
	// Total-return flags (a flag asks for a reason to hold, never excludes): a stock yielding at
	// least HighYieldFlagPct needs a 10-year price CAGR of at least HighYieldMinPriceCAGRPct; net
	// yield plus the 5-year dividend CAGR should reach MinYieldPlusGrowthPct.
	HighYieldFlagPct         *float64 `yaml:"high_yield_flag_pct,omitempty" json:"high_yield_flag_pct,omitempty"`
	HighYieldMinPriceCAGRPct *float64 `yaml:"high_yield_min_price_cagr_pct,omitempty" json:"high_yield_min_price_cagr_pct,omitempty"`
	MinYieldPlusGrowthPct    *float64 `yaml:"min_yield_plus_growth_pct,omitempty" json:"min_yield_plus_growth_pct,omitempty"`
	// Safe-dividend limits.
	MaxPayoutPct     *float64 `yaml:"max_payout_pct,omitempty" json:"max_payout_pct,omitempty"`
	MaxREITPayoutPct *float64 `yaml:"max_reit_payout_pct,omitempty" json:"max_reit_payout_pct,omitempty"`
	MaxDebtToFCF     *float64 `yaml:"max_debt_to_fcf,omitempty" json:"max_debt_to_fcf,omitempty"`
	MinCover         *float64 `yaml:"min_cover,omitempty" json:"min_cover,omitempty"`
	// Portfolio rules.
	ETFMinPct            *float64           `yaml:"etf_min_pct,omitempty" json:"etf_min_pct,omitempty"`
	ETFMaxPct            *float64           `yaml:"etf_max_pct,omitempty" json:"etf_max_pct,omitempty"`
	WeightCapPct         *float64           `yaml:"weight_cap_pct,omitempty" json:"weight_cap_pct,omitempty"` // per stock; funds are looked through
	IncomeCapPct         *float64           `yaml:"income_cap_pct,omitempty" json:"income_cap_pct,omitempty"`
	ETFIncomeCapPct      *float64           `yaml:"etf_income_cap_pct,omitempty" json:"etf_income_cap_pct,omitempty"`
	MoonshotIncomeCapPct *float64           `yaml:"moonshot_income_cap_pct,omitempty" json:"moonshot_income_cap_pct,omitempty"`
	CurrencyIncomeCapPct map[string]float64 `yaml:"currency_income_cap_pct,omitempty" json:"currency_income_cap_pct,omitempty"` // e.g. CAD: 25
	FeeMaxPct            *float64           `yaml:"fee_max_pct,omitempty" json:"fee_max_pct,omitempty"`                         // monthly trading cost
	YoCYear10Pct         *float64           `yaml:"yoc_year10_pct,omitempty" json:"yoc_year10_pct,omitempty"`                   // net yield on the start money at year 10
	PriceGrowthPct       *float64           `yaml:"price_growth_pct,omitempty" json:"price_growth_pct,omitempty"`               // the assumption, and its target
	// PriceGrowthTolerancePct is how far under price_growth_pct the weighted assumption may sit.
	PriceGrowthTolerancePct *float64 `yaml:"price_growth_tolerance_pct,omitempty" json:"price_growth_tolerance_pct,omitempty"`
	// A dividend growth filled from history is the lowest of the 10-, 5- and 3-year CAGRs times
	// ProjectionGrowthHaircut (unset: 1), capped at ProjectionGrowthCapPct (unset: no cap).
	ProjectionGrowthCapPct  *float64 `yaml:"projection_growth_cap_pct,omitempty" json:"projection_growth_cap_pct,omitempty"`
	ProjectionGrowthHaircut *float64 `yaml:"projection_growth_haircut,omitempty" json:"projection_growth_haircut,omitempty"`
	// StockReinvestCurrencies are income currencies reinvested into the stocks bought in them
	// rather than into funds (a currency with no suitable fund); they need no fund line.
	StockReinvestCurrencies []string `yaml:"stock_reinvest_currencies,omitempty" json:"stock_reinvest_currencies,omitempty"`
	// The switch to income.
	SwitchYears         []int    `yaml:"switch_years,omitempty" json:"switch_years,omitempty"`
	SwitchSellPct       *float64 `yaml:"switch_sell_pct,omitempty" json:"switch_sell_pct,omitempty"`
	SwitchStockSharePct *float64 `yaml:"switch_stock_share_pct,omitempty" json:"switch_stock_share_pct,omitempty"`
	// BondCapPct is the most of the portfolio value the switch puts in the country pack's bonds.
	BondCapPct *float64 `yaml:"bond_cap_pct,omitempty" json:"bond_cap_pct,omitempty"`
	// Buying on weakness (read by the agents): a fund this far below its 52-week high, or a stock
	// this far below, gets this multiple of its normal monthly amount.
	DoubleDownETFDropPct   *float64 `yaml:"double_down_etf_drop_pct,omitempty" json:"double_down_etf_drop_pct,omitempty"`
	DoubleDownStockDropPct *float64 `yaml:"double_down_stock_drop_pct,omitempty" json:"double_down_stock_drop_pct,omitempty"`
	DoubleDownMultiple     *float64 `yaml:"double_down_multiple,omitempty" json:"double_down_multiple,omitempty"`
	// The screen.
	IncomeCurrencies []string          `yaml:"income_currencies,omitempty" json:"income_currencies,omitempty"` // the home currency is always allowed
	ExcludedTickers  map[string]string `yaml:"excluded_tickers,omitempty" json:"excluded_tickers,omitempty"`   // ticker: why; also looked for inside funds
	// ExcludedFundWords drop a fund whose name or asset type contains one (e.g. bond, money market).
	ExcludedFundWords []string `yaml:"excluded_fund_words,omitempty" json:"excluded_fund_words,omitempty"`
	// LikedTickers are names the investor wants on every screen whatever the cut.
	LikedTickers []string `yaml:"liked_tickers,omitempty" json:"liked_tickers,omitempty"`
	// Custom holds keys a private layer's own rules read; the engine ignores it.
	Custom map[string]any `yaml:"custom,omitempty" json:"custom,omitempty"`
}

// Empty reports a strategy with nothing set.
func (s Strategy) Empty() bool { return reflect.DeepEqual(s, Strategy{}) }

// Unset lists the strategy keys with no value (custom aside), in file order.
func (s Strategy) Unset() []string {
	var out []string
	v := reflect.ValueOf(s)
	t := v.Type()
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name == "" || name == "custom" {
			continue
		}
		if v.Field(i).IsZero() {
			out = append(out, name)
		}
	}
	return out
}

// UnsetNote is what a skipped rule says.
const UnsetNote = "not set in your strategy (" + StrategyFile + "; /profile writes it from the questionnaire)"

// LoadStrategy reads data/profile/strategy.yaml; a missing file is an empty strategy.
func LoadStrategy(dataDir string) (Strategy, error) {
	var s Strategy
	p := filepath.Join(dataDir, StrategyFile)
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	// Strict: a misspelled key (yoc_year_10_pct) is an error, never a limit silently dropped.
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil && !errors.Is(err, io.EOF) {
		return s, fmt.Errorf("%s: %w (the /profile skill lists every key; a private layer's own keys go under custom:)", p, err)
	}
	return s, nil
}
