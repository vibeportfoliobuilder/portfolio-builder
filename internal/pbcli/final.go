package pbcli

// final.go - pb final from-file: the Portfolio page's final mix (the artifact db document, or the
// file its "Download final mix" button saves) into the run's allocation_final.yaml, by the rule in
// /portfolio-run, step 4.5.

import (
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
)

// FinalDoc is the page's final mix: final/<run> in the artifact db, or the downloaded JSON.
type FinalDoc struct {
	Run          string      `json:"run" yaml:"run"`
	Mix          string      `json:"mix" yaml:"mix"`
	Edited       bool        `json:"edited" yaml:"edited"`
	StartAmount  float64     `json:"start_amount" yaml:"start_amount"`
	Monthly      *float64    `json:"monthly" yaml:"monthly"`
	Years        int         `json:"years" yaml:"years"`
	InflationPct float64     `json:"inflation_pct" yaml:"inflation_pct"`
	Lines        []FinalLine `json:"lines" yaml:"lines"`
	// Switch is the page's switch-to-income inputs; absent, the source mix's own are kept.
	Switch  *rules.SwitchParams `json:"switch,omitempty" yaml:"switch,omitempty"`
	SavedAt string              `json:"saved_at" yaml:"saved_at"`
}

// FinalLine is one line the investor chose.
type FinalLine struct {
	Ticker            string   `json:"ticker" yaml:"ticker"`
	Kind              string   `json:"kind" yaml:"kind"`
	Bucket            string   `json:"bucket" yaml:"bucket"`
	WeightPct         float64  `json:"weight_pct" yaml:"weight_pct"`
	DividendGrowthPct *float64 `json:"dividend_growth_pct" yaml:"dividend_growth_pct"`
	PriceGrowthPct    *float64 `json:"price_growth_pct" yaml:"price_growth_pct"`
}

// readDBDoc reads a JSON or YAML document as the ArtifactData tool or the page's download saves it:
// either the fields themselves or wrapped in {data: {...}}.
func readDBDoc(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if json.Valid(b) {
		var wrap struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(b, &wrap) == nil && len(wrap.Data) > 0 && wrap.Data[0] == '{' {
			return json.Unmarshal(wrap.Data, v)
		}
		return json.Unmarshal(b, v)
	}
	return yaml.Unmarshal(b, v)
}

func finalCmd(e *env) *cobra.Command {
	c := &cobra.Command{Use: "final", Short: "The run's allocation_final.yaml from the Portfolio page's final mix"}
	var run string
	from := &cobra.Command{
		Use:   "from-file <final.json>",
		Short: "Write data/runs/<run>/allocation_final.yaml from the saved or downloaded final mix",
		Long: `Reads the final mix the Portfolio page saved (ArtifactData get collection final, or the JSON its
"Download final mix" button saves when the page has no database) and writes data/runs/<run>/allocation_final.yaml:
the header from start_amount, monthly, years and inflation_pct (currency: the home currency; start_year and price_growth_pct
from the mix it came from), each line's ticker, kind, bucket and weight_pct, the cited fields the source mix
states for that ticker (yield, withholding, currencies, moonshot ...), and dividend_growth_pct and
price_growth_pct only where they differ from the source mix's filled line (an investor's override), and
the page's switch-to-income inputs as the switch: block (else the source mix's own).
A ticker not on data/watchlist.yaml is reported and not written. Then run pb portfolio check on it.
Example:
  pb final from-file data/runs/<date>/final/<date>.json --run <date>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var f FinalDoc
			if err := readDBDoc(args[0], &f); err != nil {
				return fmt.Errorf("%s: %w", args[0], err)
			}
			r := cmp.Or(run, f.Run, e.today())
			if len(f.Lines) == 0 {
				return fmt.Errorf("%s has no lines", args[0])
			}
			a, skipped, src := e.finalAllocation(f, r)
			p, err := e.writeYAML(a, "runs", r, "allocation_final.yaml")
			if err != nil {
				return err
			}
			return e.printJSON(map[string]any{"file": p, "source_mix": src, "edited": f.Edited, "lines": len(a.Lines), "not_on_watchlist": skipped})
		},
	}
	from.Flags().StringVar(&run, "run", "", "run date (default: the document's run, else today)")
	c.AddCommand(from)
	return c
}

// finalAllocation applies /portfolio-run step 4.5; it returns the allocation, the tickers left out and the
// source mix file it compared against ("" when none was found).
func (e *env) finalAllocation(f FinalDoc, run string) (rules.Allocation, []string, string) {
	var src rules.Allocation
	srcFile := ""
	if f.Mix != "" && f.Mix != "saved" {
		if ok, _ := e.readYAML(&src, "runs", run, "allocation_"+f.Mix+".yaml"); ok {
			srcFile = "allocation_" + f.Mix + ".yaml"
		}
	}
	cp := src
	cp.Lines = slices.Clone(src.Lines) // fill writes into the lines it is given
	filled := e.fill(cp)
	w, _ := e.readWatchlist()
	onList := func(t string) bool {
		return len(w.Items) == 0 || slices.ContainsFunc(w.Items, func(it WatchItem) bool { return strings.EqualFold(it.Ticker, t) })
	}
	a := rules.Allocation{AsOf: e.today(), Currency: pack.Home(), StartAmount: f.StartAmount, Years: f.Years, InflationPct: f.InflationPct,
		StartYear: src.StartYear, PriceGrowthPct: src.PriceGrowthPct}
	if f.Monthly != nil {
		a.Monthly = *f.Monthly
	}
	a.Switch = src.Switch
	if f.Switch != nil {
		a.Switch = f.Switch
	}
	var skipped []string
	for _, l := range f.Lines {
		if !onList(l.Ticker) {
			skipped = append(skipped, l.Ticker)
			continue
		}
		var out rules.Line
		i := slices.IndexFunc(src.Lines, func(x rules.Line) bool { return strings.EqualFold(x.Ticker, l.Ticker) })
		if i >= 0 {
			out = src.Lines[i] // the cited fields the mix states for this ticker
			out.DividendGrowthPct, out.PriceGrowthPct = nil, nil
		}
		out.Ticker, out.Kind, out.Bucket, out.WeightPct = l.Ticker, cmp.Or(l.Kind, out.Kind), cmp.Or(l.Bucket, out.Bucket), l.WeightPct
		var fl *rules.Line
		if i >= 0 {
			fl = &filled.Lines[i]
		}
		out.DividendGrowthPct = override(l.DividendGrowthPct, fl, func(x *rules.Line) *float64 { return x.DividendGrowthPct }, src, i, true)
		out.PriceGrowthPct = override(l.PriceGrowthPct, fl, func(x *rules.Line) *float64 { return x.PriceGrowthPct }, src, i, false)
		a.Lines = append(a.Lines, out)
	}
	return a, skipped, srcFile
}

// override keeps the page's rate only where it differs from the source mix's filled line; a rate
// the mix stated itself is kept as stated.
func override(page *float64, filled *rules.Line, get func(*rules.Line) *float64, src rules.Allocation, i int, div bool) *float64 {
	var stated *float64
	if i >= 0 {
		if div {
			stated = src.Lines[i].DividendGrowthPct
		} else {
			stated = src.Lines[i].PriceGrowthPct
		}
	}
	if page == nil {
		return stated
	}
	if filled != nil {
		v := get(filled)
		// The page shows an absent rate as 0, so a 0 against an absent rate is no override.
		if (v != nil && math.Abs(*v-*page) < 1e-9) || (v == nil && *page == 0) {
			return stated
		}
	}
	v := *page
	return &v
}
