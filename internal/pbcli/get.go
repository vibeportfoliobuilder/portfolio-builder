package pbcli

// get.go - pb get: read fields out of data/ YAML files, so the analyst agents need no grep, sed or
// awk through Bash. Read-only.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func getCmd(e *env) *cobra.Command {
	var watch string
	c := &cobra.Command{
		Use:   "get <file under data/> [field ...] | --watchlist <file name> [field ...]",
		Short: "Read fields from data/ YAML files (dotted paths), one file or every watchlist name",
		Long: `Prints fields of a YAML file under data/ as JSON (or a Markdown table with --md). A field is a dotted
path: rules.withholding.rate_pct, dividend_growth_pct.5y, price.monthly_closes; a list index is a
number (years.0.fiscal_year). With no field the whole file is printed.
With --watchlist <name>, the file is read for every watchlist name (data/stocks/<T>/<name>, or
data/etfs/<T>/<name> for a fund) and one row per name is printed. Read-only.
Examples:
  pb get stocks/KO/derived.yaml yield_pct rules.withholding
  pb get --watchlist derived.yaml yield_pct rules.safe_dividend payout_pct --md
  pb get runs/<date>/check_final.yaml report.rules`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if watch != "" {
				return e.getWatchlist(watch, args)
			}
			if len(args) == 0 {
				return fmt.Errorf("name a file under data/, or pass --watchlist <file name>")
			}
			v, err := e.readUnder(args[0])
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return e.printJSON(v)
			}
			out := map[string]any{}
			for _, f := range args[1:] {
				out[f] = dig(v, f)
			}
			if e.md {
				var rows [][]string
				for _, f := range args[1:] {
					rows = append(rows, []string{f, cell(out[f])})
				}
				_, err := fmt.Fprint(e.out, mdTable([]string{"Field", "Value"}, rows))
				return err
			}
			return e.printJSON(out)
		},
	}
	c.Flags().StringVar(&watch, "watchlist", "", "read this file name for every watchlist name")
	return c
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

func (e *env) getWatchlist(name string, fields []string) error {
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("--watchlist takes a file name such as derived.yaml")
	}
	w, err := e.readWatchlist()
	if err != nil {
		return err
	}
	type row struct {
		Ticker string         `json:"ticker"`
		Values map[string]any `json:"values,omitempty"`
		Error  string         `json:"error,omitempty"`
	}
	var rows []row
	for _, it := range w.Items {
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

// dig walks a dotted path through maps and lists; absent is nil.
func dig(v any, path string) any {
	for _, k := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[k]
		case []any:
			var i int
			if _, err := fmt.Sscanf(k, "%d", &i); err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
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
