package pbcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// dig's list steps: negative indexes, slices, * and filters; the rest of the path maps over a list.
func TestDigListSteps(t *testing.T) {
	doc := map[string]any{
		"years": []any{
			map[string]any{"fiscal_year": 2027, "statements": map[string]any{"revenue": 1}},
			map[string]any{"fiscal_year": 2028, "statements": map[string]any{"revenue": 2}},
			map[string]any{"fiscal_year": 2029, "statements": map[string]any{"revenue": 3}},
		},
		"commentary": []any{
			map[string]any{"title": "Economic Moat", "text": "wide.ish"},
			map[string]any{"title": "Bulls Say", "text": "b"},
		},
		"a.b": 7,
	}
	cases := map[string]any{
		"years.0.fiscal_year":                  2027,
		"years.-1.fiscal_year":                 2029,
		"years.-3.fiscal_year":                 2027,
		"years.-4":                             nil,
		"years.3":                              nil,
		"years.x":                              nil,
		"years.-2:.fiscal_year":                []any{2028, 2029},
		"years.0:2.statements.revenue":         []any{1, 2},
		"years.:1.fiscal_year":                 []any{2027},
		"years.5:":                             []any{},
		"years.*.fiscal_year":                  []any{2027, 2028, 2029},
		"years.*.nope":                         []any{nil, nil, nil},
		"years[fiscal_year=2028].statements":   []any{map[string]any{"revenue": 2}},
		"commentary[title=economic moat].text": []any{"wide.ish"},
		"commentary.[title=Bulls Say].text":    []any{"b"},
		"commentary[title=None].text":          []any{},
		"commentary.*.title":                   []any{"Economic Moat", "Bulls Say"},
		"years[fiscal_year=2029][statements.revenue=3].fiscal_year": []any{2029},
		"nope[x=1]": nil,
	}
	for path, want := range cases {
		if got := dig(doc, path); !reflect.DeepEqual(got, want) {
			t.Errorf("dig %s = %#v, want %#v", path, got, want)
		}
	}
}

func getData(t *testing.T) string {
	t.Helper()
	data := t.TempDir()
	copyTree(t, "facts", data)
	_ = os.WriteFile(filepath.Join(data, "watchlist.yaml"), []byte("as_of: x\nsource: t\nitems:\n  - {ticker: GROW, kind: stock}\n  - {ticker: FUND, kind: etf}\n  - {ticker: PLAIN, kind: stock}\n"), 0o644)
	return data
}

// pb get <T> <file.yaml> reads stocks/<T>/ or etfs/<T>/; a real file under data/ still wins.
func TestGetTickerShorthand(t *testing.T) {
	data := getData(t)
	out := run(t, data, "get", "GROW", "financial_data.yaml", "years.-1.fiscal_year", "years.-2:.statements.revenue")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if m["years.-1.fiscal_year"] != float64(2029) || !reflect.DeepEqual(m["years.-2:.statements.revenue"], []any{float64(1210), float64(1331)}) {
		t.Errorf("%s", out)
	}
	if out := run(t, data, "get", "fund", "financial_data.yaml", "ticker"); !strings.Contains(out, `"ticker": "FUND"`) {
		t.Errorf("fund shorthand: %s", out)
	}
	if out := run(t, data, "get", "watchlist.yaml", "items.-1.ticker"); !strings.Contains(out, `"items.-1.ticker": "PLAIN"`) {
		t.Errorf("a file under data/ with fields: %s", out)
	}
	root := NewRoot(&strings.Builder{})
	root.SetArgs([]string{"--data", data, "get", "NOPE", "financial_data.yaml"})
	root.SetErr(&strings.Builder{})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "stocks/NOPE/financial_data.yaml") {
		t.Errorf("unknown ticker: %v", err)
	}
}

// --tickers keeps watchlist rows; an unknown one is an error; an empty error is left out.
func TestGetWatchlistTickers(t *testing.T) {
	data := getData(t)
	out := run(t, data, "get", "--watchlist", "derived.yaml", "--tickers", "plain,GROW", "ticker")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(rows) != 2 || rows[0]["ticker"] != "GROW" || rows[1]["ticker"] != "PLAIN" {
		t.Fatalf("%s", out)
	}
	if _, ok := rows[0]["error"]; ok {
		t.Errorf("an empty error is printed: %s", out)
	}
	if _, ok := rows[1]["values"]; ok || rows[1]["error"] != "no derived.yaml" {
		t.Errorf("a missing file row: %s", out)
	}
	root := NewRoot(&strings.Builder{})
	root.SetArgs([]string{"--data", data, "get", "--watchlist", "derived.yaml", "--tickers", "GROW,ZZZ"})
	root.SetErr(&strings.Builder{})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "ZZZ") {
		t.Errorf("unknown --tickers name: %v", err)
	}
}

// printJSON is compact: one top-level element or key per line, and still JSON.
func TestPrintJSONCompact(t *testing.T) {
	var b strings.Builder
	e := &env{out: &b}
	if err := e.printJSON([]any{map[string]any{"a": 1, "b": []any{1, 2}}, "x<y"}); err != nil {
		t.Fatal(err)
	}
	if want := "[\n{\"a\":1,\"b\":[1,2]},\n\"x<y\"\n]\n"; b.String() != want {
		t.Errorf("list: %q, want %q", b.String(), want)
	}
	b.Reset()
	_ = e.printJSON(map[string]any{"z": map[string]any{"k": "v"}, "a": nil})
	if want := "{\n\"a\": null,\n\"z\": {\"k\":\"v\"}\n}\n"; b.String() != want {
		t.Errorf("object: %q, want %q", b.String(), want)
	}
	b.Reset()
	_ = e.printJSON([]any{})
	_ = e.printJSON(map[string]any{})
	_ = e.printJSON(3)
	if b.String() != "[]\n{}\n3\n" {
		t.Errorf("empty and scalar: %q", b.String())
	}
}
