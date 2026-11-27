package pbcli

// get.go - pb get: read fields out of data/ YAML files, so the analyst agents need no grep, sed or
// awk through Bash. Read-only.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func getCmd(e *env) *cobra.Command {
	var watch, tickers string
	c := &cobra.Command{
		Use:   "get <file under data/> [field ...] | <T> <file.yaml> [field ...] | --watchlist <file name> [--tickers T,…] [field ...]",
		Short: "Read fields from data/ YAML files (dotted paths), one file or every watchlist name",
		Long: `Prints fields of a YAML file under data/ as JSON (or a Markdown table with --md). A field is a dotted
path: rules.withholding.rate_pct, dividend_growth_pct.5y, price.monthly_closes. On a list a step is
  an index        years.0 (the first), years.-1 (the last; negative counts from the end)
  a slice         years.-3: (the last three), years.0:3 (Python-style; the result is a list)
  every element   years.*.fiscal_year
  a filter        commentary[title=Economic Moat] (field equals value, case-insensitive)
After a slice, * or a filter the rest of the path applies to each element and the result is a list.
financial_data.yaml years[] runs OLDEST first (years.0 can be 1983): years.-1 is the latest year.
With no field the whole file is printed.
<T> <file.yaml> is short for stocks/<T>/<file.yaml> (or etfs/<T>/<file.yaml> for a fund).
With --watchlist <name>, the file is read for every watchlist name (data/stocks/<T>/<name>, or
data/etfs/<T>/<name> for a fund) and one row per name is printed; --tickers keeps only those names.
Read-only.
Examples:
  pb get KO financial_data.yaml years.-1.fiscal_year years.-1.statements.revenue
  pb get KO financial_data.yaml 'years.-4:.statements.fcf' 'years[fiscal_year=2025].statements'
  pb get stocks/KO/derived.yaml yield_pct rules.withholding
  pb get --watchlist derived.yaml --tickers KO,O yield_pct rules.safe_dividend payout_pct --md
  pb get runs/<date>/check_final.yaml report.rules`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if watch != "" {
				return e.getWatchlist(watch, tickers, args)
			}
			if tickers != "" {
				return fmt.Errorf("--tickers goes with --watchlist; for one name use pb get <T> <file.yaml>")
			}
			if len(args) == 0 {
				return fmt.Errorf("name a file under data/, or pass --watchlist <file name>")
			}
			rel, fields, err := e.getTarget(args)
			if err != nil {
				return err
			}
			v, err := e.readUnder(rel)
			if err != nil {
				return err
			}
			if len(fields) == 0 {
				return e.printJSON(v)
			}
			out := map[string]any{}
			for _, f := range fields {
				out[f] = dig(v, f)
			}
			if e.md {
				var rows [][]string
				for _, f := range fields {
					rows = append(rows, []string{f, cell(out[f])})
				}
				_, err := fmt.Fprint(e.out, mdTable([]string{"Field", "Value"}, rows))
				return err
			}
			return e.printJSON(out)
		},
	}
	c.Flags().StringVar(&watch, "watchlist", "", "read this file name for every watchlist name")
	c.Flags().StringVar(&tickers, "tickers", "", "with --watchlist: only these names (comma-separated)")
	return c
}

// getTarget resolves the ticker shorthand: "<T> <file.yaml> fields…" reads stocks/<T>/<file.yaml>
// (or etfs/<T>/…) when the first argument has no slash and is not itself a file under data/.
func (e *env) getTarget(args []string) (rel string, fields []string, err error) {
	first := args[0]
	if len(args) < 2 || strings.ContainsAny(first, `/\`) || !strings.HasSuffix(args[1], ".yaml") || strings.ContainsAny(args[1], `/\`) {
		return first, args[1:], nil
	}
	if _, err := os.Stat(e.path(filepath.Clean(first))); err == nil {
		return first, args[1:], nil
	}
	for _, dir := range []string{"stocks", "etfs"} {
		p := filepath.Join(dir, dirName(first), args[1])
		if _, err := os.Stat(e.path(p)); err == nil {
			return p, args[2:], nil
		}
	}
	return "", nil, fmt.Errorf("%s %s: neither stocks/%[3]s/%[2]s nor etfs/%[3]s/%[2]s is under %[4]s", first, args[1], dirName(first), e.data)
}

// readUnder reads a YAML file by its path under data/; a path that leaves data/ is refused.
func (e *env) readUnder(rel string) (any, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s: give a path under the data folder", rel)
	}
	if first := strings.Split(filepath.ToSlash(clean), "/")[0]; first == "secrets" {
		return nil, fmt.Errorf("%s: data/secrets is never read by pb get", rel)
	}
	var v any
	ok, err := e.readYAML(&v, clean)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s: no such file under %s", rel, e.data)
	}
	return dropNaN(v), nil
}

func (e *env) getWatchlist(name, only string, fields []string) error {
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("--watchlist takes a file name such as derived.yaml")
	}
	w, err := e.readWatchlist()
	if err != nil {
		return err
	}
	items, err := pickItems(w.Items, only)
	if err != nil {
		return err
	}
	// printJSON goes through YAML, so the yaml tags decide what is left out.
	type row struct {
		Ticker string         `json:"ticker" yaml:"ticker"`
		Values map[string]any `json:"values,omitempty" yaml:"values,omitempty"`
		Error  string         `json:"error,omitempty" yaml:"error,omitempty"`
	}
	var rows []row
	for _, it := range items {
		dir := "stocks"
		if it.Kind == kindETF {
			dir = "etfs"
		}
		r := row{Ticker: it.Ticker}
		v, err := e.readUnder(filepath.Join(dir, dirName(it.Ticker), name))
		if err != nil {
			r.Error = "no " + name
			rows = append(rows, r)
			continue
		}
		r.Values = map[string]any{}
		if len(fields) == 0 {
			r.Values[name] = v
		}
		for _, f := range fields {
			r.Values[f] = dig(v, f)
		}
		rows = append(rows, r)
	}
	if e.md {
		var t [][]string
		for _, r := range rows {
			line := []string{r.Ticker}
			for _, f := range fields {
				if r.Error != "" {
					line = append(line, r.Error)
					continue
				}
				line = append(line, cell(r.Values[f]))
			}
			t = append(t, line)
		}
		_, err := fmt.Fprint(e.out, mdTable(append([]string{"Ticker"}, fields...), t))
		return err
	}
	return e.printJSON(rows)
}

// pickItems keeps the watchlist rows a --tickers list names, in watchlist order; a name not on the
// watchlist is an error. An empty list keeps every row.
func pickItems(items []WatchItem, only string) ([]WatchItem, error) {
	if strings.TrimSpace(only) == "" {
		return items, nil
	}
	want := map[string]bool{}
	for _, t := range strings.Split(only, ",") {
		if t = strings.ToUpper(strings.TrimSpace(t)); t != "" {
			want[t] = true
		}
	}
	var out []WatchItem
	var all []string
	for _, it := range items {
		t := strings.ToUpper(it.Ticker)
		all = append(all, t)
		if want[t] {
			out = append(out, it)
			delete(want, t)
		}
	}
	if len(want) > 0 {
		var miss []string
		for t := range want {
			miss = append(miss, t)
		}
		sort.Strings(miss)
		return nil, fmt.Errorf("not on the watchlist: %s (the watchlist has %s)", strings.Join(miss, ", "), strings.Join(all, ", "))
	}
	return out, nil
}

// dig walks a dotted path through maps and lists; absent is nil. On a list a step is an index
// (negative counts from the end), a slice "a:b", "*" or a filter "[field=value]" (also written
// after a key: "commentary[title=Economic Moat]"); after a slice, "*" or a filter the rest of the
// path applies to each element and the result is a list.
func dig(v any, path string) any { return digSteps(v, splitPath(path)) }

func digSteps(v any, steps []string) any {
	for i, k := range steps {
		switch x := v.(type) {
		case map[string]any:
			if y, ok := x[k]; ok {
				v = y
				continue
			}
			name, filters, ok := keyFilters(k)
			if !ok {
				return nil
			}
			y, ok := x[name]
			if !ok {
				return nil
			}
			if len(filters) == 0 {
				v = y
				continue
			}
			list, ok := y.([]any)
			if !ok {
				return nil
			}
			return eachRest(filterList(list, filters), steps[i+1:])
		case []any:
			if k == "*" {
				return eachRest(x, steps[i+1:])
			}
			if lo, hi, ok := sliceBounds(k, len(x)); ok {
				return eachRest(x[lo:hi], steps[i+1:])
			}
			if name, filters, ok := keyFilters(k); ok && name == "" && len(filters) > 0 {
				return eachRest(filterList(x, filters), steps[i+1:])
			}
			n, err := strconv.Atoi(k)
			if err != nil {
				return nil
			}
			if n < 0 {
				n += len(x)
			}
			if n < 0 || n >= len(x) {
				return nil
			}
			v = x[n]
		default:
			return nil
		}
	}
	return v
}

// eachRest applies the rest of a path to every element; the result is always a list.
func eachRest(list []any, rest []string) []any {
	out := make([]any, 0, len(list))
	for _, el := range list {
		out = append(out, digSteps(el, rest))
	}
	return out
}

// splitPath splits a path on the dots outside brackets, so a filter value may hold a dot.
func splitPath(path string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range path {
		switch r {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 {
				out = append(out, path[start:i])
				start = i + 1
			}
		}
	}
	return append(out, path[start:])
}

// keyFilters reads "name[f=v][g=w]" (name may be empty); ok is false when there is no bracket or
// one is malformed.
func keyFilters(k string) (name string, filters [][2]string, ok bool) {
	i := strings.IndexByte(k, '[')
	if i < 0 || !strings.HasSuffix(k, "]") {
		return "", nil, false
	}
	name, rest := k[:i], k[i:]
	for rest != "" {
		end := strings.IndexByte(rest, ']')
		if !strings.HasPrefix(rest, "[") || end < 0 {
			return "", nil, false
		}
		f, val, found := strings.Cut(rest[1:end], "=")
		if !found || strings.TrimSpace(f) == "" {
			return "", nil, false
		}
		filters = append(filters, [2]string{strings.TrimSpace(f), strings.TrimSpace(val)})
		rest = rest[end+1:]
	}
	return name, filters, true
}

// filterList keeps the elements whose field equals the value (case-insensitive, numbers as written).
func filterList(list []any, filters [][2]string) []any {
	out := []any{}
	for _, el := range list {
		keep := true
		for _, f := range filters {
			got := dig(el, f[0])
			if got == nil || !strings.EqualFold(fmt.Sprint(got), f[1]) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, el)
		}
	}
	return out
}

// sliceBounds reads a Python-style slice "a:b" (either side may be empty or negative) for a list of n.
func sliceBounds(k string, n int) (lo, hi int, ok bool) {
	a, b, found := strings.Cut(k, ":")
	if !found {
		return 0, 0, false
	}
	bound := func(s string, def int) (int, bool) {
		if s == "" {
			return def, true
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		if i < 0 {
			i += n
		}
		return min(max(i, 0), n), true
	}
	lo, ok1 := bound(a, 0)
	hi, ok2 := bound(b, n)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return lo, max(lo, hi), true
}

// cell prints a value for a Markdown table: scalars as they are, maps and lists compactly.
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return "–"
	case map[string]any:
		ks := make([]string, 0, len(x))
		for k := range x {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		var parts []string
		for _, k := range ks {
			parts = append(parts, k+": "+cell(x[k]))
		}
		return strings.Join(parts, "; ")
	case []any:
		var parts []string
		for _, y := range x {
			parts = append(parts, cell(y))
		}
		return strings.Join(parts, ", ")
	}
	return strings.ReplaceAll(fmt.Sprint(v), "|", "/")
}
