package pbcli

import (
	"encoding/json"
	"strings"
	"testing"
)

// pb facts: the last N years, the repaired TTM, the gate figures, and a source on every figure.
func TestFactsGates(t *testing.T) {
	data := getData(t)
	out := run(t, data, "facts", "GROW", "fund", "--years", "2")
	var docs []map[string]any
	if err := json.Unmarshal([]byte(out), &docs); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(docs) != 2 {
		t.Fatalf("%s", out)
	}
	g := docs[0]
	years := g["years"].([]any)
	if len(years) != 2 || years[0].(map[string]any)["fy"] != float64(2028) || years[1].(map[string]any)["fy"] != float64(2029) {
		t.Fatalf("years: %v", years)
	}
	last := years[1].(map[string]any)
	for k, want := range map[string]any{"revenue": 1331.0, "op_margin_pct": 20.0, "fcf_margin_pct": 20.0, "diluted_shares": 48.0, "dps": 1.0,
		"source": "financial_data.yaml years[fiscal_year=2029].statements", "dps_source": "financial_data.yaml dividend_history.by_fy[fiscal_year=2029].total"} {
		if last[k] != want {
			t.Errorf("FY2029 %s = %v, want %v", k, last[k], want)
		}
	}
	ttm := g["ttm"].(map[string]any)
	if ttm["source"] != "derived.yaml ttm_used" || ttm["revenue"] != 1400.0 || ttm["fcf_margin_pct"] != 15.0 || ttm["dps"] != 1.1 {
		t.Errorf("ttm: %v", ttm)
	}
	gates := g["gates"].(map[string]any)
	gate := func(k string) map[string]any { return gates[k].(map[string]any) }
	if c := gate("gate1_revenue_cagr_3y_pct"); c["value"] != 10.0 || c["passed"] != true || !strings.Contains(c["source"].(string), "years[fiscal_year=2026].statements.revenue") {
		t.Errorf("cagr: %v", c)
	}
	if r := gate("gate2_rule_of_40"); r["value"] != 30.0 || r["passed"] != false {
		t.Errorf("rule of 40: %v", r)
	}
	if n := gate("gate1_tests"); n["value"] != 3.0 {
		t.Errorf("tests: %v", n)
	}
	for k := range gates {
		if gate(k)["source"] == "" {
			t.Errorf("%s has no source", k)
		}
	}
	if docs[1]["note"] != "no statements (fund)" {
		t.Errorf("fund: %v", docs[1])
	}
}

// A stock with gaps says DATA UNAVAILABLE and fails only what the data shows; --md has a source column.
func TestFactsGapsAndMarkdown(t *testing.T) {
	data := getData(t)
	md := run(t, data, "facts", "--watchlist", "--tickers", "PLAIN,FUND", "--md")
	for _, want := range []string{
		"## PLAIN (stocks/PLAIN/financial_data.yaml, XTS)",
		"| 2029 | 450 | -5 | -1.11 | – | – | – | – | 10 | 400 | years[fiscal_year=2029].statements |",
		"gate1_revenue_cagr_3y_pct: –; DATA UNAVAILABLE",
		"gate1_profitable: –; DATA UNAVAILABLE",
		"gate1_cash_vs_debt: [cash_and_st_inv > total_debt: false]",
		"gate1_tests: 0 passed of 1 known",
		"gate2_rule_of_40: –; DATA UNAVAILABLE",
		"## FUND\n\nno statements (fund)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	if strings.Contains(md, "GROW") {
		t.Errorf("--tickers did not filter:\n%s", md)
	}
}
