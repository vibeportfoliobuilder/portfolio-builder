package pbcli

// scenario_test.go - pb scenario compounds the agent's assumptions from the files' start values
// (a made-up name in XTS), and refuses incomplete or misspelled assumptions.

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scenarioEnv(t *testing.T, derived, financial string) (*env, *bytes.Buffer) {
	t.Helper()
	data := t.TempDir()
	dir := filepath.Join(data, "stocks", "TSTB")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"derived.yaml": derived, "financial_data.yaml": financial} {
		if body == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	return &env{data: data, out: &out, now: func() time.Time { return time.Date(2030, 3, 10, 0, 0, 0, 0, time.UTC) }}, &out
}

const (
	scenarioDerived = `as_of: "2030-03-10"
ticker: TSTB
price: 100
price_currency: XTS
yield_pct: 4
dividend_growth_pct: {5y: 5}
rules:
  withholding: {country: TL, rate_pct: 15, tier: A}
`
	scenarioFinancial = `price_currency: XTS
price: {current: 99}
dividend_history: {currency: XTS, ttm_per_share: 4}
`
)

func runScenarioCmd(t *testing.T, e *env, args ...string) error {
	t.Helper()
	c := scenarioCmd(e)
	c.SetArgs(args)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	return c.Execute()
}

func TestScenarioCompoundsFromTheFiles(t *testing.T) {
	e, out := scenarioEnv(t, scenarioDerived, scenarioFinancial)
	err := runScenarioCmd(t, e, "TSTB", "--case", "base:div_growth=10,price_cagr=5", "--case", "bear:div_growth=0,price_cagr=-10,cut_year=3,cut_pct=50")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Names []ScenarioOut `json:"names"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	o := v.Names[0]
	if o.Price.Value != 100 || !strings.HasPrefix(o.Price.Source, "derived.yaml: price") || o.DPS.Value != 4 ||
		o.DPS.Source != "financial_data.yaml: dividend_history.ttm_per_share" || o.WithholdingPct.Value != 15 {
		t.Fatalf("start values %+v %+v %+v", o.Price, o.DPS, o.WithholdingPct)
	}
	if len(o.Cases) != 2 || o.Cases[0].Case != "base" || o.Cases[1].Case != "bear" {
		t.Fatalf("cases %+v", o.Cases)
	}
	years := func(c ScenarioResult) map[int]ScenarioYear {
		m := map[int]ScenarioYear{}
		for _, y := range c.Years {
			m[y.Year] = y
		}
		return m
	}
	base, bear := years(o.Cases[0]), years(o.Cases[1])
	if len(base) != 5 {
		t.Errorf("years shown %v, want 0 1 3 5 10", base)
	}
	near := func(what string, got *float64, want float64) {
		t.Helper()
		if got == nil || math.Abs(*got-want) > 0.011 {
			t.Errorf("%s = %v, want %.4f", what, got, want)
		}
	}
	// Year 1 earns today's run rate; year 3 two years of growth.
	near("base y1 dps", base[1].DPS, 4)
	near("base y3 dps", base[3].DPS, 4*1.1*1.1)
	near("base y3 gross yoc", base[3].GrossYoCPct, 4.84)
	near("base y3 net yoc", base[3].NetYoCPct, 4.84*0.85)
	near("base y3 cumulative net", base[3].CumNetDiv, (4+4.4+4.84)*0.85)
	p10 := 100 * math.Pow(1.05, 10)
	if math.Abs(base[10].Price-p10) > 0.01 {
		t.Errorf("base y10 price %v, want %.2f", base[10].Price, p10)
	}
	cum10 := 0.0
	for n := 1; n <= 10; n++ {
		cum10 += 4 * math.Pow(1.1, float64(n-1)) * 0.85
	}
	near("base y10 total return", base[10].TotalReturnPct, math.Round((p10+cum10-100)*10)/10)
	// The cut halves the dividend from year 3 on.
	near("bear y1 dps", bear[1].DPS, 4)
	near("bear y3 dps", bear[3].DPS, 2)
	near("bear y5 cumulative net", bear[5].CumNetDiv, (4+4+2+2+2)*0.85)
	near("bear y10 price", ptr(bear[10].Price), 100*math.Pow(0.9, 10))
}

func TestScenarioFileAndMarkdown(t *testing.T) {
	e, out := scenarioEnv(t, scenarioDerived, scenarioFinancial)
	e.md = true
	f := filepath.Join(t.TempDir(), "a.yaml")
	os.WriteFile(f, []byte("years: 5\nnames:\n- ticker: tstb\n  cases:\n    bear: {div_growth: 0, price_cagr: 0}\n    bull: {div_growth: 8, price_cagr: 9}\n"), 0o644)
	if err := runScenarioCmd(t, e, "--file", f); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"## TSTB", "- price 100 XTS (derived.yaml: price", "- withholding 15% (derived.yaml: rules.withholding.rate_pct (TL))",
		"| bull (g 8%, price 9%) | 0 | 100.00 | 4.0000 | 4.00 | 3.40 | 0.00 | 0.0 | – |", "| bear (g 0%, price 0%) | 0 |", "|  | 5 | 100.00 | 4.0000 | 4.00 | 3.40 | 17.00 | 17.0 |"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if strings.Index(s, "| bull") > strings.Index(s, "| bear") {
		t.Error("bull is not listed before bear")
	}
}

func TestScenarioRefusesBadAssumptions(t *testing.T) {
	e, _ := scenarioEnv(t, scenarioDerived, scenarioFinancial)
	for _, args := range [][]string{
		{"TSTB", "--case", "base:div_growth=5"},                                     // no price_cagr
		{"TSTB", "--case", "base:div_grwoth=5,price_cagr=3"},                        // misspelled key
		{"TSTB", "--case", "base:div_growth=5,price_cagr=3,cut_year=12,cut_pct=30"}, // cut past the horizon
		{"TSTB"}, // no case
		{"NOPE", "--case", "base:div_growth=5,price_cagr=3"}, // no files
	} {
		if err := runScenarioCmd(t, e, args...); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	f := filepath.Join(t.TempDir(), "a.yaml")
	os.WriteFile(f, []byte("names:\n- ticker: TSTB\n  cases:\n    base: {div_growth: 5, price_cagr: 3, cutyear: 2}\n"), 0o644)
	if err := runScenarioCmd(t, e, "--file", f); err == nil {
		t.Error("a misspelled key in --file was accepted")
	}
}

func TestScenarioFallsBackWhenTheLedgerIsInAnotherCurrency(t *testing.T) {
	fin := strings.Replace(scenarioFinancial, "{currency: XTS,", "{currency: XXX,", 1)
	e, out := scenarioEnv(t, strings.Replace(scenarioDerived, "  withholding: {country: TL, rate_pct: 15, tier: A}\n", "  group: x\n", 1), fin)
	if err := runScenarioCmd(t, e, "TSTB", "--case", "base:div_growth=0,price_cagr=0"); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Names []ScenarioOut `json:"names"`
	}
	json.Unmarshal(out.Bytes(), &v)
	o := v.Names[0]
	if o.DPS == nil || o.DPS.Value != 4 || o.DPS.Source != "derived.yaml: yield_pct × price" || o.WithholdingPct != nil || len(o.Gaps) != 2 {
		t.Fatalf("%+v %+v", o.DPS, o.Gaps)
	}
	if y := o.Cases[0].Years[1]; y.NetYoCPct != nil || y.GrossYoCPct == nil || *y.TotalReturnPct != 0 {
		t.Errorf("without withholding the net columns must be absent: %+v", y)
	}
}
