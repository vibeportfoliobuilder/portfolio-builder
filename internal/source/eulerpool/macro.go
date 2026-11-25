package eulerpool

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Macro is the compact file `pb ep macro` writes: rates, FX against the home currency, CPI,
// calendar, country risk.
type Macro struct {
	AsOf        string        `yaml:"as_of"`
	Source      string        `yaml:"source"`
	Countries   []string      `yaml:"countries"`
	DataGaps    []string      `yaml:"data_gaps,omitempty"`
	PolicyRates []PolicyRate  `yaml:"policy_rates,omitempty"`
	USRates     []SeriesValue `yaml:"us_rates,omitempty"`
	FX          []FXRate      `yaml:"fx,omitempty"`
	CPI         []CPIRow      `yaml:"cpi,omitempty"`
	Calendar    []Event       `yaml:"calendar,omitempty"`
	CountryRisk []CountryRisk `yaml:"country_risk,omitempty"`
}

// PolicyRate is a central-bank rate in percent.
type PolicyRate struct {
	Area string  `yaml:"area"`
	Name string  `yaml:"name"`
	Rate float64 `yaml:"rate"`
	AsOf string  `yaml:"as_of"`
}

// SeriesValue is one US rate series' latest value in percent.
type SeriesValue struct {
	Series string  `yaml:"series"`
	Title  string  `yaml:"title"`
	Value  float64 `yaml:"value"`
	AsOf   string  `yaml:"as_of"`
}

// FXRate is home currency per one unit of a currency (Pair "USD/<home>"), crossed through the
// ECB's EUR fixings.
type FXRate struct {
	Pair     string   `yaml:"pair"`
	Rate     float64  `yaml:"rate"`
	AsOf     string   `yaml:"as_of"`
	Change1m *float64 `yaml:"change_1m_pct,omitempty"`
	Change1y *float64 `yaml:"change_1y_pct,omitempty"`
}

// CPIRow is the latest year-on-year inflation print for one area.
type CPIRow struct {
	Area      string   `yaml:"area"`
	Name      string   `yaml:"name"`
	Reference string   `yaml:"reference,omitempty"`
	Value     float64  `yaml:"value"`
	Previous  *float64 `yaml:"previous,omitempty"`
	Released  string   `yaml:"released"`
}

// Event is one upcoming calendar release.
type Event struct {
	Date      string   `yaml:"date"`
	Country   string   `yaml:"country"`
	Name      string   `yaml:"name"`
	Previous  *float64 `yaml:"previous,omitempty"`
	Consensus *float64 `yaml:"consensus,omitempty"`
}

// CountryRisk is the vendor's Damodaran-style row; premiums in percent.
type CountryRisk struct {
	Country  string   `yaml:"country"`
	Name     string   `yaml:"name"`
	Currency string   `yaml:"currency,omitempty"`
	Rating   string   `yaml:"rating,omitempty"`
	ERP      *float64 `yaml:"equity_risk_premium,omitempty"`
	CRP      *float64 `yaml:"country_risk_premium,omitempty"`
}

// MacroOptions sets the calendar window, the home currency the FX is quoted in and the areas always
// covered (the country pack's); zero values take the defaults.
type MacroOptions struct {
	LookbackDays int
	AheadDays    int
	// Home is the currency every FX pair is quoted in (default EUR).
	Home string
	// BaseAreas are always covered besides the requested countries (default US, EU).
	BaseAreas []string
	// Currencies are FX pairs always quoted besides USD, EUR and the areas' own (the investor's
	// income currencies).
	Currencies []string
}

// defaultBaseAreas are covered when the options name none: the US and the euro area (the market
// cap is read in USD; the ECB fixings are EUR-based).
var defaultBaseAreas = []string{"US", "EU"}

// calendarKeep are the release names (substrings) kept in the upcoming calendar.
var calendarKeep = []string{"Inflation Rate YoY", "Core Inflation Rate YoY", "Interest Rate Decision", "GDP Growth Rate", "Unemployment Rate", "Non Farm Payrolls", "Retail Sales YoY", "Manufacturing PMI"}

type wireEvent struct {
	Timestamp Num    `json:"timestamp"`
	Country   string `json:"country_code"`
	Name      string `json:"name"`
	Reference string `json:"reference_date"`
	Actual    Num    `json:"actual_value"`
	Previous  Num    `json:"previous_value"`
	Consensus Num    `json:"consensus_value"`
}

type wireRisk struct {
	Code2    string `json:"code2"`
	Country  string `json:"country"`
	Currency string `json:"currency_code"`
	Rating   string `json:"rating"`
	ERP      Num    `json:"equity_risk_premium"`
	CRP      Num    `json:"country_risk_premium"`
}

type ecbObs struct {
	Date string `json:"date"`
	Rate Num    `json:"rate"`
}

// Macro builds the macro file for the given ISO-2 countries (plus the base areas), six to eight requests.
func (c *Client) Macro(ctx context.Context, countries []string, opt MacroOptions) (*Macro, error) {
	now := c.now().UTC()
	opt.LookbackDays = cmp.Or(opt.LookbackDays, 60)
	opt.AheadDays = cmp.Or(opt.AheadDays, 28)
	opt.Home = strings.ToUpper(cmp.Or(opt.Home, "EUR"))
	areas := slices.Clone(defaultBaseAreas)
	if len(opt.BaseAreas) > 0 {
		areas = nil
		for _, a := range opt.BaseAreas {
			if a = strings.ToUpper(strings.TrimSpace(a)); a != "" && !slices.Contains(areas, a) {
				areas = append(areas, a)
			}
		}
	}
	for _, cc := range countries {
		cc = strings.ToUpper(strings.TrimSpace(cc))
		if cc != "" && !slices.Contains(areas, cc) {
			areas = append(areas, cc)
		}
	}
	m := &Macro{AsOf: day(now), Source: "eulerpool", Countries: areas}
	s := &fetchState{d: &Dossier{}}

	var risks []wireRisk
	s.soft(ctx, "country risk", c.getJSON(ctx, "macro/country-risk", nil, TTLMacro*14, &risks))
	ccy := map[string]string{"EU": "EUR"}
	for _, r := range risks {
		ccy[r.Code2] = r.Currency
		if slices.Contains(areas, r.Code2) {
			m.CountryRisk = append(m.CountryRisk, CountryRisk{Country: r.Code2, Name: r.Country, Currency: r.Currency, Rating: r.Rating, ERP: r.ERP.Ptr(), CRP: r.CRP.Ptr()})
		}
	}
	slices.SortFunc(m.CountryRisk, func(a, b CountryRisk) int { return strings.Compare(a.Country, b.Country) })

	var us struct {
		Data []struct {
			Series string `json:"series_id"`
			Date   string `json:"date"`
			Value  Num    `json:"value"`
			Title  string `json:"title"`
		} `json:"data"`
	}
	s.soft(ctx, "US interest rates", c.getJSON(ctx, "interest-rates/rates", nil, TTLMacro, &us))
	for _, r := range us.Data {
		if r.Value.OK {
			m.USRates = append(m.USRates, SeriesValue{Series: r.Series, Title: r.Title, Value: r.Value.V, AsOf: r.Date})
		}
		if r.Series == "DFF" && r.Value.OK {
			m.PolicyRates = append(m.PolicyRates, PolicyRate{Area: "US", Name: "Fed funds effective", Rate: r.Value.V, AsOf: r.Date})
		}
	}
	var ecb struct {
		Data []struct {
			Date string `json:"date"`
			Type string `json:"rate_type"`
			Rate Num    `json:"rate"`
		} `json:"data"`
	}
	s.soft(ctx, "ECB key rates", c.getJSON(ctx, "ecb/key-rates", url.Values{"limit": {"12"}}, TTLMacro, &ecb))
	seenECB := map[string]bool{}
	for _, r := range ecb.Data {
		if r.Rate.OK && !seenECB[r.Type] {
			seenECB[r.Type] = true
			m.PolicyRates = append(m.PolicyRates, PolicyRate{Area: "EU", Name: "ECB " + strings.ReplaceAll(r.Type, "_", " "), Rate: *round(&r.Rate.V, 4), AsOf: r.Date})
		}
	}

	c.macroFX(ctx, s, m, areas, ccy, opt.Home, opt.Currencies, now)
	c.macroCalendar(ctx, s, m, areas, ccy, opt, now)
	c.euroAreaCPI(ctx, s, m, now)
	slices.SortStableFunc(m.PolicyRates, func(a, b PolicyRate) int { return strings.Compare(a.Area, b.Area) })
	m.DataGaps = s.d.DataGaps
	if s.fatal != nil {
		return nil, s.fatal
	}
	return m, nil
}

// macroFX crosses ECB EUR fixings into home per USD, EUR, the extra currencies and each area's currency.
func (c *Client) macroFX(ctx context.Context, s *fetchState, m *Macro, areas []string, ccy map[string]string, home string, extra []string, now time.Time) {
	want := []string{"USD", "EUR"}
	for _, k := range extra {
		if k = strings.ToUpper(strings.TrimSpace(k)); k != "" && !slices.Contains(want, k) {
			want = append(want, k)
		}
	}
	for _, a := range areas {
		if k := ccy[a]; k != "" && !slices.Contains(want, k) {
			want = append(want, k)
		}
	}
	series := func(cur string) map[string]float64 {
		var v struct {
			Data []ecbObs `json:"data"`
		}
		if !s.soft(ctx, "ECB "+cur+" fixings", c.getJSON(ctx, "ecb/exchange-rates/"+seg(cur), url.Values{"limit": {"400"}}, TTLMacro, &v)) {
			return nil
		}
		out := map[string]float64{}
		for _, o := range v.Data {
			if o.Rate.OK && o.Rate.V > 0 {
				out[o.Date] = o.Rate.V
			}
		}
		return out
	}
	// The ECB fixes every currency per EUR; home per cur is (home per EUR) / (cur per EUR), with EUR
	// itself at 1.
	var hs map[string]float64
	if home != "EUR" {
		if hs = series(home); len(hs) == 0 {
			return
		}
	}
	for _, cur := range want {
		if cur == home {
			continue
		}
		var os map[string]float64
		if cur != "EUR" {
			if os = series(cur); len(os) == 0 {
				continue
			}
		}
		cross := map[string]float64{}
		switch {
		case hs != nil && os != nil:
			for d, h := range hs {
				if o, ok := os[d]; ok {
					cross[d] = h / o
				}
			}
		case hs != nil:
			for d, h := range hs {
				cross[d] = h
			}
		default:
			for d, o := range os {
				cross[d] = 1 / o
			}
		}
		var dates []string
		for d := range cross {
			dates = append(dates, d)
		}
		if len(dates) == 0 {
			continue
		}
		slices.Sort(dates)
		last := dates[len(dates)-1]
		at := func(back time.Time) *float64 {
			b := day(back)
			i, _ := slices.BinarySearch(dates, b)
			if i < len(dates) && dates[i] == b {
				return f(cross[dates[i]])
			}
			if i == 0 {
				return nil
			}
			return f(cross[dates[i-1]])
		}
		lt, _ := parseDay(last)
		chg := func(old *float64) *float64 {
			if old == nil || *old == 0 {
				return nil
			}
			return round(f((cross[last] / *old - 1)*100), 2)
		}
		m.FX = append(m.FX, FXRate{Pair: cur + "/" + home, Rate: sigFig(cross[last], 6), AsOf: last, Change1m: chg(at(lt.AddDate(0, -1, 0))), Change1y: chg(at(lt.AddDate(-1, 0, 0)))})
		if now.Sub(lt) > 7*24*time.Hour {
			s.gap("eulerpool: the ECB %s/%s cross ends %s", cur, home, last)
		}
	}
}

// macroCalendar reads one window: past releases give CPI and policy rates, future ones the calendar.
func (c *Client) macroCalendar(ctx context.Context, s *fetchState, m *Macro, areas []string, ccy map[string]string, opt MacroOptions, now time.Time) {
	var codes []string
	for _, a := range areas {
		if a != "EU" {
			codes = append(codes, strings.ToLower(a))
		}
	}
	q := url.Values{"start": {day(now.AddDate(0, 0, -opt.LookbackDays))}, "end": {day(now.AddDate(0, 0, opt.AheadDays))}, "countries": {strings.Join(codes, ",")}}
	var evs []wireEvent
	if !s.soft(ctx, "macro calendar", c.getJSON(ctx, "macro/calendar", q, TTLMacro, &evs)) {
		return
	}
	// The vendor sends each release twice, once without the actual; keep the richer row.
	type key struct {
		t       int64
		country string
		name    string
	}
	best := map[key]wireEvent{}
	for _, e := range evs {
		k := key{int64(e.Timestamp.V), e.Country, e.Name}
		if old, ok := best[k]; !ok || (!old.Actual.OK && e.Actual.OK) || (!old.Consensus.OK && e.Consensus.OK && !old.Actual.OK) {
			best[k] = e
		}
	}
	keys := make([]key, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b key) int {
		return cmp.Or(cmp.Compare(a.t, b.t), strings.Compare(a.country, b.country), strings.Compare(a.name, b.name))
	})
	nowMs := now.UnixMilli()
	cpi := map[string]CPIRow{}
	policy := map[string]PolicyRate{}
	for _, k := range keys {
		e := best[k]
		when := time.UnixMilli(k.t).UTC()
		area := strings.ToUpper(e.Country)
		if k.t < nowMs {
			switch {
			case e.Actual.OK && (e.Name == "Inflation Rate YoY" || strings.HasPrefix(e.Name, "Inflation Rate YoY ")):
				cpi[area] = CPIRow{Area: area, Name: e.Name, Reference: refMonth(e.Reference, when), Value: e.Actual.V, Previous: e.Previous.Ptr(), Released: day(when)}
			case e.Actual.OK && e.Name == "Interest Rate Decision":
				policy[area] = PolicyRate{Area: area, Name: "policy rate", Rate: e.Actual.V, AsOf: day(when)}
			}
			continue
		}
		if keep(e.Name) {
			m.Calendar = append(m.Calendar, Event{Date: day(when), Country: area, Name: e.Name, Previous: e.Previous.Ptr(), Consensus: e.Consensus.Ptr()})
		}
	}
	for _, a := range areas {
		if r, ok := cpi[a]; ok {
			m.CPI = append(m.CPI, r)
		} else if a != "EU" {
			s.gap("eulerpool: no inflation print for %s in the last %d days of the macro calendar", a, opt.LookbackDays)
		}
		if r, ok := policy[a]; ok && a != "US" {
			m.PolicyRates = append(m.PolicyRates, r)
		} else if !ok && a != "US" && ccy[a] != "EUR" {
			s.gap("eulerpool: no policy-rate decision for %s in the last %d days of the macro calendar", a, opt.LookbackDays)
		}
	}
}

// euroAreaCPI takes euro-area HICP from the ECB mirror, which stopped at 2025-12.
func (c *Client) euroAreaCPI(ctx context.Context, s *fetchState, m *Macro, now time.Time) {
	var rows []struct {
		Key   string `json:"series_key"`
		Name  string `json:"name"`
		Date  string `json:"latest_date"`
		Value Num    `json:"latest_value"`
	}
	if !s.soft(ctx, "ECB latest series", c.getJSON(ctx, "macro/latest/ecb", nil, TTLMacro, &rows)) {
		return
	}
	for _, r := range rows {
		if r.Key != "ICP/M.U2.N.000000.4.ANR" || !r.Value.OK {
			continue
		}
		m.CPI = append(m.CPI, CPIRow{Area: "EU", Name: "HICP Overall (Euro Area)", Reference: r.Date[:min(7, len(r.Date))], Value: r.Value.V, Released: r.Date})
		if t, ok := parseDay(r.Date); ok && now.Sub(t) > 90*24*time.Hour {
			s.gap("eulerpool: euro-area HICP is from %s; the vendor's ECB mirror stopped there and the calendar has no euro-area code, so DE and FR prints stand in", r.Date)
		}
	}
}

func keep(name string) bool {
	for _, k := range calendarKeep {
		if strings.Contains(name, k) {
			return true
		}
	}
	return false
}

// refMonth turns the vendor's reference month ("9") into "2026-09" relative to the release.
func refMonth(ref string, released time.Time) string {
	n, err := strconv.Atoi(strings.TrimSpace(ref))
	if err != nil || n < 1 || n > 12 {
		return ref
	}
	y := released.Year()
	if n > int(released.Month()) {
		y--
	}
	return fmt.Sprintf("%d-%02d", y, n)
}

// sigFig rounds v to n significant digits: a rate of 0.0000571 (IDR in EUR) keeps its digits,
// where a fixed 4 decimals would round it to 0.0001.
func sigFig(v float64, n int) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	p := math.Pow(10, float64(n)-math.Ceil(math.Log10(math.Abs(v))))
	return math.Round(v*p) / p
}
