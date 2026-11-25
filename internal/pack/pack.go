// Package pack holds what the engine must know about where the investor lives (a country pack)
// and where they buy (a broker pack). Packs are data: YAML files under data/packs/, written at
// setup by the /country-research and /broker-connector skills, or a built-in pack registered by name. The
// engine reads the active pair through Active; with no configuration that is the made-up
// Exampleland and Example Broker, so every command and test runs without a real country.
package pack

import (
	"embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"go.yaml.in/yaml/v4"
)

// Country is a country pack: the investor's home currency, inflation target, the withholding a
// dividend suffers at source for a resident, a transaction tax on the home market and the bonds
// the switch to income buys.
type Country struct {
	Code     string `yaml:"code" json:"code"`
	Name     string `yaml:"name" json:"name"`
	Currency string `yaml:"currency" json:"currency"`
	// InflationPct is the long-run inflation the dividend growth must beat after year 10.
	InflationPct  float64 `yaml:"inflation_pct" json:"inflation_pct"`
	InflationNote string  `yaml:"inflation_note,omitempty" json:"inflation_note,omitempty"`
	// Withholding is the rate a source country keeps from a dividend paid to a resident, percent by
	// ISO alpha-2. A country missing here has no stated rate (never a zero).
	Withholding map[string]float64 `yaml:"withholding" json:"withholding"`
	// REITWithholding is the rate a REIT from that country suffers where it differs (a UK REIT's
	// property-income distribution).
	REITWithholding map[string]float64 `yaml:"reit_withholding,omitempty" json:"reit_withholding,omitempty"`
	// FundWithholding is what a fund's distribution loses by the fund's domicile (an Irish or
	// Luxembourg UCITS fund usually pays gross: 0). A domicile missing here falls back to
	// Withholding; with neither, the rate is unknown, never a zero.
	FundWithholding map[string]float64 `yaml:"fund_withholding,omitempty" json:"fund_withholding,omitempty"`
	// Unrated are codes no single rate is honest for; they draw a dash.
	Unrated []string `yaml:"unrated,omitempty" json:"unrated,omitempty"`
	// TransactionTax is charged on a buy in the home market, nil where there is none.
	TransactionTax *Levy `yaml:"transaction_tax,omitempty" json:"transaction_tax,omitempty"`
	// Bonds are what the switch to income buys with part of the proceeds.
	Bonds Bonds `yaml:"bonds" json:"bonds"`
	// Account names the tax-advantaged account the plan is held in ("" when none).
	Account string `yaml:"account,omitempty" json:"account,omitempty"`
	// MacroAreas are the areas pb ep macro always covers besides the watchlist's countries.
	MacroAreas []string `yaml:"macro_areas" json:"macro_areas"`
	// Sources are where the pack's figures were read, with the date.
	Sources []string `yaml:"sources,omitempty" json:"sources,omitempty"`
}

// Levy is a transaction tax: RatePct of a buy in Market (a fee market key), capped at Cap and not
// charged on a buy of ExemptUpTo or less (both in the home currency; zero means none).
type Levy struct {
	Label      string  `yaml:"label" json:"label"`
	Market     string  `yaml:"market" json:"market"`
	RatePct    float64 `yaml:"rate_pct" json:"rate_pct"`
	Cap        float64 `yaml:"cap,omitempty" json:"cap,omitempty"`
	ExemptUpTo float64 `yaml:"exempt_up_to,omitempty" json:"exempt_up_to,omitempty"`
}

// Bonds is the switch's bond sleeve: a name, its currency and the yields it is shown at. How much
// of the portfolio goes into it is the investor's (strategy bond_cap_pct).
type Bonds struct {
	Name      string    `yaml:"name" json:"name"`
	Currency  string    `yaml:"currency" json:"currency"`
	YieldsPct []float64 `yaml:"yields_pct" json:"yields_pct"`
	TaxFree   bool      `yaml:"tax_free" json:"tax_free"`
}

// Broker is a broker pack: what an order costs per market, the FX cost on income that changes
// currency, and where the broker's withholding differs from the country table.
type Broker struct {
	Key  string `yaml:"key" json:"key"`
	Name string `yaml:"name" json:"name"`
	// FXCostPct is charged on money converted into another currency.
	FXCostPct float64 `yaml:"fx_cost_pct" json:"fx_cost_pct"`
	// WithholdingOverrides replace the country table's rate where the broker withholds otherwise
	// (it files no treaty claim, say); REITWithholding is the rate for a REIT from that country.
	WithholdingOverrides map[string]float64 `yaml:"withholding_overrides,omitempty" json:"withholding_overrides,omitempty"`
	REITWithholding      map[string]float64 `yaml:"reit_withholding,omitempty" json:"reit_withholding,omitempty"`
	// CreditsLineCurrency means the broker credits a dividend in the currency of the line it
	// trades, not the one the company declares (a Canadian stock on a US line pays USD).
	CreditsLineCurrency bool `yaml:"credits_line_currency,omitempty" json:"credits_line_currency,omitempty"`
	// LevyNoExemption charges the home transaction tax on every buy, small ones included.
	LevyNoExemption bool `yaml:"levy_no_exemption,omitempty" json:"levy_no_exemption,omitempty"`
	// Markets prices one buy order per market key.
	Markets map[string]Market `yaml:"markets" json:"markets"`
	// DefaultETFMarket is where a fund line is bought when the mix names no market.
	DefaultETFMarket string `yaml:"default_etf_market" json:"default_etf_market"`
	// MarketByCountry is where a stock is bought by its country when the mix names no market.
	MarketByCountry map[string]string `yaml:"market_by_country" json:"market_by_country"`
	// DefaultMarket is the market of last resort.
	DefaultMarket string   `yaml:"default_market" json:"default_market"`
	Sources       []string `yaml:"sources,omitempty" json:"sources,omitempty"`

	// fees prices an order where a built-in pack needs more than a commission formula.
	fees FeeModel
}

// Market is one venue's price for a buy: a fixed fee plus PctFee of the order, bounded by Min
// and Max (zero: none), and a transaction tax of TaxPct the venue's country charges. Offered
// false means the broker does not take orders there.
type Market struct {
	Name     string  `yaml:"name" json:"name"`
	Currency string  `yaml:"currency" json:"currency"`
	Kind     string  `yaml:"kind,omitempty" json:"kind,omitempty"` // share | etf
	Fixed    float64 `yaml:"fixed,omitempty" json:"fixed,omitempty"`
	PctFee   float64 `yaml:"pct_fee,omitempty" json:"pct_fee,omitempty"`
	Min      float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max      float64 `yaml:"max,omitempty" json:"max,omitempty"`
	TaxPct   float64 `yaml:"tax_pct,omitempty" json:"tax_pct,omitempty"`
	TaxLabel string  `yaml:"tax_label,omitempty" json:"tax_label,omitempty"`
	Offered  *bool   `yaml:"offered,omitempty" json:"offered,omitempty"`
}

// Part is one line of a quote: the commission or a tax.
type Part struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}

// Quote is what one buy costs at the broker, or why it cannot be bought.
type Quote struct {
	Offered bool    `json:"offered"`
	Why     string  `json:"why,omitempty"`
	Parts   []Part  `json:"parts,omitempty"`
	Total   float64 `json:"total"`
}

// FeeModel prices a buy of amount (in the market's currency) in a market. A built-in broker
// pack registers one when its schedule is more than a commission formula.
type FeeModel interface {
	Currency(market string) (string, bool)
	Quote(market string, amount float64) Quote
}

// Currency is the currency a market is priced in.
func (b Broker) Currency(market string) (string, bool) {
	if b.fees != nil {
		return b.fees.Currency(market)
	}
	m, ok := b.Markets[market]
	return m.Currency, ok && m.Currency != ""
}

// Quote prices a buy of amount (in the market's currency) in market, its venue tax included.
func (b Broker) Quote(market string, amount float64) Quote {
	if b.fees != nil {
		return b.fees.Quote(market, amount)
	}
	m, ok := b.Markets[market]
	if !ok {
		return Quote{Why: "market " + market + " is not in the broker pack"}
	}
	if m.Offered != nil && !*m.Offered {
		return Quote{Why: b.Name + " does not offer this market"}
	}
	fee := m.Fixed + amount*m.PctFee/100
	if m.Min > 0 {
		fee = max(fee, m.Min)
	}
	if m.Max > 0 {
		fee = min(fee, m.Max)
	}
	q := Quote{Offered: true, Parts: []Part{{"commission", fee}}, Total: fee}
	if m.TaxPct > 0 {
		label := m.TaxLabel
		if label == "" {
			label = fmt.Sprintf("transaction tax %g%%", m.TaxPct)
		}
		t := amount * m.TaxPct / 100
		q.Parts = append(q.Parts, Part{label, t})
		q.Total += t
	}
	return q
}

// WithFees returns the pack with a fee model in place of its market formulas.
func (b Broker) WithFees(f FeeModel) Broker {
	b.fees = f
	return b
}

// Levy is the home transaction tax on a buy of amount (in the home currency: the cap and the
// exemption are home-currency figures) in market, nil where none. A caller pricing a venue whose
// Market already states a TaxPct does not add it again.
func (s Set) Levy(market string, amount float64) []Part {
	t := s.Country.TransactionTax
	if t == nil || market != t.Market || amount <= 0 {
		return nil
	}
	if !s.Broker.LevyNoExemption && amount <= t.ExemptUpTo {
		return nil
	}
	v := amount * t.RatePct / 100
	if t.Cap > 0 {
		v = min(v, t.Cap)
	}
	label := t.Label
	if label == "" {
		label = fmt.Sprintf("transaction tax %g%%", t.RatePct)
	}
	return []Part{{label, v}}
}

// Rate is the withholding a dividend sourced in country suffers at this broker: the broker's
// REIT or override rate, else the country pack's. ok is false where no rate is stated.
func (s Set) Rate(country string, reit bool) (float64, bool) {
	code := strings.ToUpper(strings.TrimSpace(country))
	if reit {
		if r, ok := s.Broker.REITWithholding[code]; ok {
			return r, true
		}
		if r, ok := s.Country.REITWithholding[code]; ok {
			return r, true
		}
	}
	if r, ok := s.Broker.WithholdingOverrides[code]; ok {
		return r, true
	}
	r, ok := s.Country.Withholding[code]
	return r, ok
}

// Rated reports whether a code has a stated rate.
func (s Set) Rated(code string) bool {
	_, ok := s.Rate(code, false)
	return ok
}

// Set is the active pair.
type Set struct {
	Country Country `json:"country"`
	Broker  Broker  `json:"broker"`
	// Language is the language the user reads (an ISO 639-1 code); agents' own files stay English.
	Language string `json:"language"`
	// Strategy is the investor's own (data/profile/strategy.yaml); empty until the questionnaire.
	Strategy Strategy `json:"strategy"`
}

// Config is data/config.yaml: which packs are active and the user's language.
type Config struct {
	Country  string `yaml:"country"`
	Broker   string `yaml:"broker"`
	Language string `yaml:"language,omitempty"`
}

// The names used when data/config.yaml is absent.
const (
	DefaultCountry = "exampleland"
	DefaultBroker  = "examplebroker"
)

//go:embed examples/*.yaml
var examples embed.FS

var (
	mu       sync.RWMutex
	builtins = map[string][]byte{}
	brokerFM = map[string]FeeModel{}
	active   *Set
)

func init() {
	for _, n := range []string{"country/exampleland", "broker/examplebroker"} {
		b, err := examples.ReadFile("examples/" + filepath.Base(n) + ".yaml")
		if err != nil {
			panic(err)
		}
		builtins[n] = b
	}
}

// Register adds a built-in pack: kind is "country" or "broker", name the key config.yaml uses.
// A private layer calls it from init to ship its own packs inside the binary.
func Register(kind, name string, yamlDoc []byte) {
	mu.Lock()
	defer mu.Unlock()
	builtins[strings.ToLower(kind)+"/"+strings.ToLower(strings.TrimSpace(name))] = yamlDoc
}

// RegisterFeeModel gives a built-in broker pack a fee model in place of its market formulas.
func RegisterFeeModel(name string, f FeeModel) {
	mu.Lock()
	defer mu.Unlock()
	brokerFM[strings.ToLower(strings.TrimSpace(name))] = f
}

// Names lists the built-in packs of a kind.
func Names(kind string) []string {
	mu.RLock()
	defer mu.RUnlock()
	var out []string
	for k := range maps.Keys(builtins) {
		if n, ok := strings.CutPrefix(k, kind+"/"); ok {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// Load reads data/config.yaml and the two packs it names: data/packs/<kind>/<name>.yaml first,
// else a built-in. A missing config.yaml means the examples.
func Load(dataDir string) (Set, error) {
	cfg := Config{Country: DefaultCountry, Broker: DefaultBroker, Language: "en"}
	b, err := os.ReadFile(filepath.Join(dataDir, "config.yaml"))
	switch {
	case err == nil:
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return Set{}, fmt.Errorf("%s: %w", filepath.Join(dataDir, "config.yaml"), err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return Set{}, err
	}
	return load(dataDir, cfg)
}

// load reads the two packs cfg names; an empty dataDir reads the built-ins only.
func load(dataDir string, cfg Config) (Set, error) {
	var s Set
	s.Language = cfg.Language
	if s.Language == "" {
		s.Language = "en"
	}
	if err := read(dataDir, "country", cfg.Country, DefaultCountry, &s.Country); err != nil {
		return Set{}, err
	}
	if err := read(dataDir, "broker", cfg.Broker, DefaultBroker, &s.Broker); err != nil {
		return Set{}, err
	}
	if s.Broker.Key == "" {
		s.Broker.Key = strings.ToLower(strings.TrimSpace(cmpOr(cfg.Broker, DefaultBroker)))
	}
	mu.RLock()
	if f := brokerFM[strings.ToLower(strings.TrimSpace(cmpOr(cfg.Broker, DefaultBroker)))]; f != nil {
		s.Broker.fees = f
	}
	mu.RUnlock()
	if dataDir != "" {
		st, err := LoadStrategy(dataDir)
		if err != nil {
			return Set{}, err
		}
		s.Strategy = st
	}
	return s, s.check()
}

func read(dataDir, kind, name, def string, v any) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = def
	}
	p := filepath.Join(dataDir, "packs", kind, name+".yaml")
	var b []byte
	err := os.ErrNotExist
	if dataDir != "" {
		b, err = os.ReadFile(p)
	}
	if errors.Is(err, os.ErrNotExist) {
		mu.RLock()
		b = builtins[kind+"/"+name]
		mu.RUnlock()
		if b == nil {
			return fmt.Errorf("pack: no %s pack %q (looked in %s and the built-ins: %s)", kind, name, p, strings.Join(Names(kind), ", "))
		}
		p = "built-in " + kind + "/" + name
	} else if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("pack %s: %w", p, err)
	}
	return nil
}

func (s Set) check() error {
	c := s.Country
	switch {
	case c.Currency == "":
		return fmt.Errorf("pack: country %q has no currency", c.Name)
	case c.InflationPct <= 0:
		return fmt.Errorf("pack: country %q has no inflation_pct", c.Name)
	case len(s.Broker.Markets) == 0 && s.Broker.fees == nil:
		return fmt.Errorf("pack: broker %q prices no market", s.Broker.Name)
	}
	return nil
}

// Example is the made-up pair the engine falls back to; tests use it.
func Example() Set {
	s, err := load("", Config{Country: DefaultCountry, Broker: DefaultBroker, Language: "en"})
	if err != nil {
		panic(err)
	}
	return s
}

// Active is the pair in use: the one Use set, else the example.
func Active() Set {
	mu.RLock()
	a := active
	mu.RUnlock()
	if a != nil {
		return *a
	}
	s := Example()
	mu.Lock()
	if active == nil {
		active = &s
	}
	mu.Unlock()
	return s
}

// Use makes s the active pair (pb does this once, after reading data/config.yaml).
func Use(s Set) {
	mu.Lock()
	defer mu.Unlock()
	active = &s
}

func cmpOr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// Home is the active home currency, upper case.
func Home() string { return strings.ToUpper(Active().Country.Currency) }
