package pbcli

// runjoin.go - agent files written in batches (<base>_part<N>.md, in the run folder or its parts/)
// joined into the one <base>.md that the next agent and pb report build read.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// partRe names a batch file: <base>_part<N>.md.
var partRe = regexp.MustCompile(`^(.+)_part(\d+)\.md$`)

// verdictRe is the committee chair's verdict line, "- Verdict: **WATCHLIST** — …" (any bold, bullet or
// trailing qualifier such as "(moonshot sleeve only)").
var verdictRe = regexp.MustCompile(`(?mi)^[ \t]*(?:[-*][ \t]+)?\**Verdict\**[ \t]*:[ \t]*\**[ \t]*(BUY-WORTHY|WATCHLIST|PASS)\b`)

// verdictOf finds the committee's call in a ticker's verdict section: the Verdict line, else (an
// older layout) the first verdict word anywhere. The fallback alone read a WATCHLIST name's
// "trigger to BUY-WORTHY" as its verdict.
func verdictOf(s string) string {
	if m := verdictRe.FindStringSubmatch(s); m != nil {
		return strings.ToUpper(m[1])
	}
	for _, v := range []string{"BUY-WORTHY", "WATCHLIST", "PASS"} {
		if strings.Contains(s, v) {
			return v
		}
	}
	return ""
}

// mdSection is one "## " section of a Markdown file, in file order.
type mdSection struct{ Head, Body string }

// splitOrdered is splitSections keeping the order (and duplicates) of the sections.
func splitOrdered(md string) (string, []mdSection) {
	var out []mdSection
	var intro, cur strings.Builder
	head, inIntro := "", true
	flush := func() {
		if !inIntro {
			out = append(out, mdSection{head, strings.TrimSpace(cur.String())})
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

// headTicker is the ticker a "## " heading starts with ("KO — Coca-Cola" → KO).
func headTicker(head string) string {
	return strings.ToUpper(strings.Trim(strings.Fields(head + " x")[0], "*`:,"))
}

// tickerHeadRe guesses a ticker heading when no watchlist is at hand: an upper-case first word and a dash.
var tickerHeadRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.\-]{0,11}\s+[—–-]\s`)

// isTickerHead says whether a section belongs to one name rather than to the batch.
func isTickerHead(head string, tickers map[string]bool) bool {
	if len(tickers) > 0 {
		return tickers[headTicker(head)]
	}
	return tickerHeadRe.MatchString(head)
}

// joinParts joins batch files (given in part order) into one file: the first part's title, then per
// part its intro and its non-ticker sections (demoted to "####" and labelled with the part's file),
// then every ticker section in order. This is the layout the agents write.
func joinParts(names, contents []string, tickers map[string]bool) string {
	var b, secs strings.Builder
	title := ""
	for i, c := range contents {
		intro, sections := splitOrdered(c)
		var rest []string
		for _, l := range strings.Split(intro, "\n") {
			if title == "" && strings.HasPrefix(l, "# ") {
				title = strings.TrimSpace(l)
				continue
			}
			if i > 0 && strings.HasPrefix(l, "# ") && len(rest) == 0 {
				continue // a later part's own title
			}
			rest = append(rest, l)
		}
		fmt.Fprintf(&b, "### From %s\n\n", names[i])
		if s := strings.TrimSpace(strings.Join(rest, "\n")); s != "" {
			b.WriteString(s + "\n\n")
		}
		for _, s := range sections {
			if isTickerHead(s.Head, tickers) {
				fmt.Fprintf(&secs, "## %s\n\n%s\n\n", s.Head, s.Body)
				continue
			}
			fmt.Fprintf(&b, "#### %s (%s)\n\n%s\n\n", s.Head, names[i], s.Body)
		}
	}
	if title == "" {
		title = "# " + strings.TrimSuffix(partRe.ReplaceAllString(names[0], "$1"), ".md")
	}
	return fmt.Sprintf("%s (joined from %d batch files)\n\n%s%s", title, len(contents), b.String(), strings.TrimRight(secs.String(), "\n")+"\n")
}

// runParts finds the batch files of a run matching glob, in the run folder and its parts/, grouped
// by base name (<base>.md) and sorted by part number.
func runParts(dir, glob string) map[string][]string {
	out := map[string][]string{}
	for _, d := range []string{dir, filepath.Join(dir, "parts")} {
		ms, _ := filepath.Glob(filepath.Join(d, glob))
		for _, m := range ms {
			if p := partRe.FindStringSubmatch(filepath.Base(m)); p != nil {
				out[p[1]+".md"] = append(out[p[1]+".md"], m)
			}
		}
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return partNo(out[k][i]) < partNo(out[k][j]) })
	}
	return out
}

func partNo(p string) int {
	n, _ := strconv.Atoi(partRe.FindStringSubmatch(filepath.Base(p))[2])
	return n
}

// agentFile reads an agent's file for the run: the newest whole file matching glob (never a
// _partN file), else the newest set of batch files joined. name is what the page cites.
func agentFile(dir, glob string, tickers map[string]bool) (name, content string, err error) {
	ms, _ := filepath.Glob(filepath.Join(dir, glob))
	var whole []string
	for _, m := range ms {
		if !partRe.MatchString(filepath.Base(m)) {
			whole = append(whole, m)
		}
	}
	if len(whole) > 0 {
		sort.Strings(whole)
		b, err := os.ReadFile(whole[len(whole)-1])
		return filepath.Base(whole[len(whole)-1]), string(b), err
	}
	groups := runParts(dir, strings.TrimSuffix(glob, ".md")+"_part*.md")
	if len(groups) == 0 {
		return "", "", nil
	}
	var bases []string
	for k := range groups {
		bases = append(bases, k)
	}
	sort.Strings(bases)
	base := bases[len(bases)-1]
	names, contents, err := readParts(groups[base])
	if err != nil {
		return "", "", err
	}
	return base + " (joined from " + strings.Join(names, ", ") + ")", joinParts(names, contents, tickers), nil
}

func readParts(paths []string) (names, contents []string, err error) {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, filepath.Base(p))
		contents = append(contents, string(b))
	}
	return names, contents, nil
}

// watchTickers is the watchlist's tickers in upper case (empty without a watchlist).
func (e *env) watchTickers() (map[string]bool, error) {
	w, err := e.readWatchlist()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, it := range w.Items {
		out[strings.ToUpper(it.Ticker)] = true
	}
	return out, nil
}

func runCmd(e *env) *cobra.Command {
	c := &cobra.Command{Use: "run", Short: "Helpers for a run folder (data/runs/<date>)"}
	var run string
	var force bool
	join := &cobra.Command{
		Use:   "join",
		Short: "Join batch files (<base>_partN.md) into the one <base>.md the next agent reads",
		Long: `When an agent ran the watchlist in batches it writes <base>_part1.md, _part2.md … (in the run folder
or its parts/). join writes <base>.md beside them: the first part's title, each part's intro and batch-wide
sections (as "#### <heading> (<part file>)"), then every "## <TICKER>" section in order. A <base>.md that
already exists is kept unless --force. pb report build joins the same way on the fly when <base>.md is absent.
Example:
  pb run join --run <date>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if run == "" {
				run = e.latestRun()
			}
			dir := e.path("runs", run)
			tickers, err := e.watchTickers()
			if err != nil {
				return err
			}
			type row struct {
				File  string   `yaml:"file"`
				Parts []string `yaml:"parts"`
				Kept  bool     `yaml:"kept,omitempty"`
			}
			var rows []row
			groups := runParts(dir, "*.md")
			var bases []string
			for k := range groups {
				bases = append(bases, k)
			}
			sort.Strings(bases)
			for _, base := range bases {
				names, contents, err := readParts(groups[base])
				if err != nil {
					return err
				}
				p := filepath.Join(dir, base)
				r := row{File: p, Parts: names}
				if _, err := os.Stat(p); err == nil && !force {
					r.Kept = true
				} else if err := store.WriteFileAtomic(p, []byte(joinParts(names, contents, tickers))); err != nil {
					return err
				}
				rows = append(rows, r)
			}
			return e.printJSON(map[string]any{"run": run, "joined": rows})
		},
	}
	join.Flags().StringVar(&run, "run", "", "run date (default: the newest data/runs/<date>)")
	join.Flags().BoolVar(&force, "force", false, "rewrite a <base>.md that already exists")
	c.AddCommand(join)
	return c
}
