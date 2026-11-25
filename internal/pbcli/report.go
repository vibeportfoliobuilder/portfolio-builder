package pbcli

// report.go - pb report build: everything the Artifact shows, gathered from data/ into one JSON
// object and embedded in internal/report's template. The page computes nothing it could read here,
// except what the investor edits (weights, swaps, inputs) on the Portfolio page.

import (
	"cmp"
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/broker"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/report"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/rules"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/stock/divscore"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// Mixes are the three allocations /portfolio-builder writes, plus the investor's final one.
var reportMixes = []struct{ Key, Label string }{
	{"balanced", "Balanced"}, {"income", "Income tilt"}, {"growth", "Growth tilt"}, {"final", "Final"},
}

// agentFiles are the run files whose per-ticker sections the Watchlist page shows, in run order.
var agentFiles = []struct{ Key, Label, Glob string }{
	{"screen_notes", "Screen notes (Joel)", "screen_notes.md"},
	{"moat", "Moat review (Hamilton)", "moat_review.md"},
	{"ceo", "CEO review (William)", "ceo_review.md"},
	{"scenarios", "Scenarios (Merrill, Benjamin, Ursula)", "scenarios.md"},
	{"news", "News and macro (Nellie)", "news_macro*.md"},
	{"verdicts", "Verdicts (Charlie)", "verdicts.md"},
	{"fit", "Fit (Harry)", "fit.md"},
	{"audit", "Dossier audit", "*audit.md"}, // audit.md (dossier_audit.md in older runs)
}

type reportDoc struct {
	GeneratedAt  string         `json:"generated_at"`
	Run          string         `json:"run"`
	Profile      map[string]any `json:"profile"`
	Screen       *ScreenDoc     `json:"screen,omitempty"`
	Names        []reportName   `json:"names"`
	Agents       []reportAgent  `json:"agents"`
	Macro        any            `json:"macro,omitempty"`
	Mixes        []reportMix    `json:"mixes"`
	Alternatives []rules.Line   `json:"alternatives"`
	Blueprint    string         `json:"blueprint,omitempty"`
	StressIntro  string         `json:"stress_intro,omitempty"`
	Notes        []string       `json:"notes,omitempty"`
	// Pack is the active country and broker packs: the page reads the home currency, the FX cost,
	// the bonds and the inflation target from it, so it projects on the same figures as pb.
	Pack pack.Set `json:"pack"`
	// Extra holds what registered report extensions add (a private connector's account pages).
	Extra map[string]any `json:"extra,omitempty"`
	// DividendSet says what the safety score's relative half was ranked among.
	DividendSet *divscore.Set `json:"dividend_set,omitempty"`
}

type reportName struct {
	Ticker    string            `json:"ticker"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Country   string            `json:"country,omitempty"`
	Currency  string            `json:"currency,omitempty"`
	Sector    string            `json:"sector,omitempty"`
	Derived   any               `json:"derived,omitempty"`
	Financial any               `json:"financial,omitempty"`
	HumanView any               `json:"human_view,omitempty"`
	Broker    any               `json:"broker,omitempty"`
	Edgar     any               `json:"edgar,omitempty"`
	Sections  map[string]string `json:"sections,omitempty"`
	Verdict   string            `json:"verdict,omitempty"`
	// Returns is pb ep returns' file (stocks and funds); DividendRank the score's place in the watchlist.
	Returns      any                `json:"returns,omitempty"`
	DividendRank *divscore.Rank     `json:"dividend_rank,omitempty"`
	Domicile     string             `json:"domicile,omitempty"`
	Withholding  *rules.Withholding `json:"withholding,omitempty"`
	// Favourites are the investor's names a fund holds above 0.5%.
	Favourites []Favourite `json:"favourites"`
}

type reportAgent struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	File    string `json:"file"`
	Summary string `json:"summary"`
}

type reportMix struct {
	Key        string           `json:"key"`
	Label      string           `json:"label"`
	File       string           `json:"file"`
	Allocation rules.Allocation `json:"allocation"`
	Check      rules.Report     `json:"check"`
	Stress     string           `json:"stress,omitempty"`
}

func reportCmd(e *env) *cobra.Command {
	c := &cobra.Command{Use: "report", Short: "The run's Artifact page (Screen, Watchlist, Portfolio)"}
	var run, out string
	build := &cobra.Command{
		Use:   "build",
		Short: "Gather data/ and the run's files into data/report/index.html",
		Long: `Reads data/watchlist.yaml and each name's files (derived, financial_data, human_view, broker,
edgar), data/macro.yaml, the run's screen.yaml, every agent file (split into per-ticker sections at
"## <TICKER>" headings), allocation_{balanced,income,growth,final}.yaml (each checked like pb portfolio
check) and stress.md (split per mix), and writes one self-contained page with the data embedded.
Publish it with the Artifact tool; the URL is kept in data/report/artifact_url.txt.
Example:
  pb report build --run <date>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if run == "" {
				run = e.latestRun()
			}
			doc, err := e.reportDoc(run)
			if err != nil {
				return err
			}
			html, err := report.Render(doc)
			if err != nil {
				return err
			}
			p := cmp.Or(out, e.path("report", "index.html"))
			if err := store.WriteFileAtomic(p, html); err != nil {
				return err
			}
			return e.printJSON(map[string]any{"file": p, "bytes": len(html), "run": run, "names": len(doc.Names),
				"mixes": len(doc.Mixes), "notes": doc.Notes})
		},
	}
	build.Flags().StringVar(&run, "run", "", "run date (default: the newest data/runs/<date>)")
	build.Flags().StringVar(&out, "out", "", "output file (default data/report/index.html)")
	c.AddCommand(build)
	return c
}

// latestRun is the newest data/runs/<YYYY-MM-DD>, else today.
func (e *env) latestRun() string {
	ents, _ := os.ReadDir(e.path("runs"))
	var ds []string
	for _, d := range ents {
		if d.IsDir() && regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(d.Name()) {
			ds = append(ds, d.Name())
		}
	}
	if len(ds) == 0 {
		return e.today()
	}
	sort.Strings(ds)
	return ds[len(ds)-1]
}

// readAny reads a YAML file under data/ as plain values; absent is nil.
func (e *env) readAny(parts ...string) any {
	var v any
	if ok, err := e.readYAML(&v, parts...); err != nil || !ok {
		return nil
	}
	return dropNaN(v)
}

// dropNaN removes NaN and infinite numbers (YAML .nan) from a decoded file: JSON cannot carry them,
// and an absent figure is what they mean (a quant-only fair value once broke pb report build).
func dropNaN(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, y := range x {
			if f, ok := y.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
				delete(x, k)
				continue
			}
			x[k] = dropNaN(y)
		}
	case []any:
		out := x[:0]
		for _, y := range x {
			if f, ok := y.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
				continue
			}
			out = append(out, dropNaN(y))
		}
		return out
	}
	return v
}

func (e *env) reportDoc(run string) (*reportDoc, error) {
	doc := &reportDoc{GeneratedAt: e.now().UTC().Format("2006-01-02 15:04 UTC"), Run: run, Profile: profileRules()}
	var sc ScreenDoc
	if ok, err := e.readYAML(&sc, "runs", run, "screen.yaml"); err != nil {
		return nil, err
	} else if ok {
		fillScreenTax(&sc)
		doc.Screen = &sc
	} else {
		doc.Notes = append(doc.Notes, "no screen.yaml for this run")
	}
	w, err := e.readWatchlist()
	if err != nil {
		return nil, err
	}
	tickers := map[string]bool{}
	for _, it := range w.Items {
		tickers[strings.ToUpper(it.Ticker)] = true
	}
	// Agent files: the summary above the first section, then one section per ticker.
	sections := map[string]map[string]string{}
	for _, af := range agentFiles {
		// The whole file, else its batch files (_partN.md) joined, as pb run join would.
		name, md, err := agentFile(e.path("runs", run), af.Glob, tickers)
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		summary, parts := splitSections(md)
		doc.Agents = append(doc.Agents, reportAgent{Key: af.Key, Label: af.Label, File: name, Summary: summary})
		for head, body := range parts {
			t := strings.ToUpper(strings.Trim(strings.Fields(head + " x")[0], "*`:,"))
			if tickers[t] {
				if sections[t] == nil {
					sections[t] = map[string]string{}
				}
				sections[t][af.Key] = body
			}
		}
	}
	for _, it := range w.Items {
		dir := "stocks"
		if it.Kind == kindETF {
			dir = "etfs"
		}
		t := dirName(it.Ticker)
		n := reportName{Ticker: it.Ticker, Name: it.Name, Kind: it.Kind, Country: it.Country, Currency: it.Currency, Sector: it.Sector,
			Derived: e.readAny(dir, t, "derived.yaml"), Financial: pruneFinancial(e.readAny(dir, t, "financial_data.yaml")),
			HumanView: e.readAny(dir, t, "human_view.yaml"), Broker: e.readAny(dir, t, broker.File),
			Sections: sections[strings.ToUpper(it.Ticker)]}
		if ed, ok := e.readAny(dir, t, "edgar.yaml").(map[string]any); ok {
			n.Edgar = map[string]any{"cik": ed["cik"], "filings": ed["filings"], "ffo": ed["ffo"]}
		}
		n.Verdict = verdictOf(n.Sections["verdicts"])
		n.Returns = e.readAny(dir, t, "returns.yaml")
		if it.Kind == kindETF {
			n.Domicile, n.Withholding = rules.FundDomicile(it.ISIN), rules.FundWithholding(it.ISIN)
		}
		doc.Names = append(doc.Names, n)
	}
	// The investor's names inside each fund, on the Screen and the Watchlist pages.
	var picked []string
	var pk Picks
	if readDBDoc(e.path("runs", run, "picks", run+".json"), &pk) == nil {
		picked = pk.Stocks
	}
	favs := e.favouriteNames(doc.Screen, w, picked)
	if doc.Screen != nil {
		for i := range doc.Screen.Funds {
			if f := doc.Screen.Funds[i].Facts; f != nil {
				doc.Screen.Funds[i].Favourites = favouritesIn(f.Holdings, favs)
			}
		}
	}
	for i := range doc.Names {
		if doc.Names[i].Kind == kindETF {
			doc.Names[i].Favourites = favouritesIn(e.etfHoldings(doc.Names[i].Ticker), favs)
		}
	}
	// The safety score's relative half: struck over this watchlist's stocks, stored nowhere.
	members := map[string]divscore.Member{}
	for _, it := range w.Items {
		var dv Derived
		if it.Kind == kindETF {
			continue
		}
		if ok, _ := e.readYAML(&dv, "stocks", dirName(it.Ticker), "derived.yaml"); ok && dv.DividendSafety != nil {
			m := divscore.Member{Score: *dv.DividendSafety}
			if dv.YieldPct != nil {
				m.Yield = *dv.YieldPct
			}
			members[it.Ticker] = m
		}
	}
	if len(members) > 0 {
		set := divscore.RankSet(members)
		for i := range doc.Names {
			if r, ok := set.Ranks[doc.Names[i].Ticker]; ok {
				doc.Names[i].DividendRank = &r
			}
		}
		set.Ranks = nil
		doc.DividendSet = &set
	}
	if m := e.readAny("macro.yaml"); m != nil {
		doc.Macro = m
	}
	// Mixes, each checked here exactly as pb portfolio check would, so the page shows pb's numbers.
	stressIntro, stress := "", map[string]string{}
	if b, err := os.ReadFile(e.path("runs", run, "stress.md")); err == nil {
		var parts map[string]string
		stressIntro, parts = splitSections(string(b))
		for head, body := range parts {
			h := strings.ToLower(head)
			for _, m := range reportMixes {
				if strings.Contains(h, m.Key) {
					stress[m.Key] = body
				}
			}
		}
	}
	doc.StressIntro = stressIntro
	if b, err := os.ReadFile(e.path("runs", run, "blueprint.md")); err == nil {
		doc.Blueprint = string(b)
	}
	for _, m := range reportMixes {
		p := e.path("runs", run, "allocation_"+m.Key+".yaml")
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var a rules.Allocation
		if err := yaml.Unmarshal(b, &a); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		filled, r := e.checkAllocation(a)
		withWithholding(&filled, r)
		doc.Mixes = append(doc.Mixes, reportMix{Key: m.Key, Label: m.Label, File: filepath.Base(p), Allocation: filled, Check: r, Stress: stress[m.Key]})
	}
	if len(doc.Mixes) == 0 {
		doc.Notes = append(doc.Notes, "no allocation_<mix>.yaml in this run yet")
	}
	// Alternatives: every watchlist name (and every mix line) filled the same way, for swaps and adds.
	seen := map[string]bool{}
	add := func(l rules.Line) {
		k := strings.ToUpper(l.Ticker)
		if seen[k] {
			return
		}
		seen[k] = true
		l.WeightPct = 0
		doc.Alternatives = append(doc.Alternatives, l)
	}
	for _, it := range w.Items {
		kind := rules.KindStock
		if it.Kind == kindETF {
			kind = ""
		}
		f, r := e.checkAllocation(rules.Allocation{Lines: []rules.Line{{Ticker: it.Ticker, Kind: kind, Name: it.Name, WeightPct: 100}}})
		withWithholding(&f, r)
		add(f.Lines[0])
	}
	for _, m := range doc.Mixes {
		for _, l := range m.Allocation.Lines {
			add(l)
		}
	}
	slices.SortFunc(doc.Alternatives, func(a, b rules.Line) int { return strings.Compare(a.Ticker, b.Ticker) })
	doc.Pack = pack.Active()
	for _, x := range reportExtensions {
		if err := x(e, doc); err != nil {
			return doc, err
		}
	}
	return doc, nil
}

// ReportExtension adds data for extra report pages (doc.Extra[name]) before the page is built.
type ReportExtension func(e *Env, doc *ReportDoc) error

// ReportDoc is the data the report page is built from.
type ReportDoc = reportDoc

var reportExtensions []ReportExtension

// RegisterReportExtension adds an extension; a connector calls it from init, beside
// report.RegisterPage for the page itself.
func RegisterReportExtension(f ReportExtension) { reportExtensions = append(reportExtensions, f) }

// withWithholding writes the rate the check applied onto each line that did not state one, so the
// page computes net yields with pb's rates.
func withWithholding(a *rules.Allocation, r rules.Report) {
	for i := range a.Lines {
		if a.Lines[i].WithholdingPct == nil && i < len(r.Lines) {
			w := r.Lines[i].WithholdingPct
			a.Lines[i].WithholdingPct = &w
		}
	}
}

// splitSections cuts Markdown at its "## " headings: the text before the first, and each section
// keyed by its heading text.
func splitSections(md string) (string, map[string]string) {
	out := map[string]string{}
	var head string
	var cur, intro strings.Builder
	inIntro := true
	flush := func() {
		if !inIntro {
			out[head] = strings.TrimSpace(cur.String())
		}
		cur.Reset()
	}
	for _, line := range strings.SplitAfter(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			inIntro = false
			head = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		if inIntro {
			intro.WriteString(line)
		} else {
			cur.WriteString(line)
		}
	}
	flush()
	return strings.TrimSpace(intro.String()), out
}

// pruneFinancial drops what the page never shows, to keep it small: quarters, the TTM block,
// payment lists and insider rows.
func pruneFinancial(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	delete(m, "quarters")
	delete(m, "ttm")
	delete(m, "shares")
	if d, ok := m["dividend_history"].(map[string]any); ok {
		delete(d, "payments")
	}
	if ins, ok := m["insider_transactions"].(map[string]any); ok {
		delete(ins, "recent")
	}
	if id, ok := m["identity"].(map[string]any); ok {
		if s, ok := id["description"].(string); ok && len(s) > 600 {
			id["description"] = s[:600] + " …"
		}
	}
	return m
}

// profileRules are the strategy's portfolio thresholds the Portfolio page starts its editable rules
// from; an unset one is null, and the page shows its rule without judging it.
func profileRules() map[string]any {
	st := pack.Active().Strategy
	return map[string]any{
		"etf_min_pct": st.ETFMinPct, "etf_max_pct": st.ETFMaxPct, "weight_cap_pct": st.WeightCapPct,
		"income_cap_pct": st.IncomeCapPct, "etf_income_cap_pct": st.ETFIncomeCapPct, "moonshot_income_cap_pct": st.MoonshotIncomeCapPct,
		"currency_income_cap_pct": st.CurrencyIncomeCapPct, "fee_max_pct": st.FeeMaxPct,
		"yoc_year10_pct": st.YoCYear10Pct, "price_growth_pct": st.PriceGrowthPct, "price_growth_tolerance_pct": st.PriceGrowthTolerancePct,
		"growth_after_pct": rules.InflationPct(), "inflation_pct": rules.InflationPct(),
		"start_amount": st.StartAmount, "monthly": st.Monthly, "years": 10,
	}
}
