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
	c.AddCommand(join, sectionCmd(e))
	return c
}

func sectionCmd(e *env) *cobra.Command {
	var summary bool
	var headings []string
	c := &cobra.Command{
		Use:   "section <file.md under data/> [T ...] [--summary] [--heading <prefix> ...]",
		Short: "Print only some sections of a Markdown file under data/ (a ticker's, the Summary, numbered headings)",
		Long: `Prints sections of a Markdown file under data/ as they are, so an agent reads only what it needs:
  <T> ...            the "## <T> — …" section of each ticker (an unknown ticker lists the ones found)
  --summary          the "## Summary" section
  --heading <prefix> each "##" or "###" heading whose text starts with the prefix, with its block up
                     to the next heading of the same or a higher level; a number matches up to a
                     space or a dot, so 6.1 matches "### 6.1 Groups" but not "### 6.10 Exclusions"
With no selector it prints the file's "##"/"###" headings (a table of contents). A bare file name
(moat_review.md) is read from the newest run folder when data/ has no such file. Read-only.
Examples:
  pb run section runs/<date>/moat_review.md --summary KO O
  pb run section verdicts.md MSFT
  pb run section profile/investor_profile.md --heading 1. --heading 6.1 --heading 9.
  pb run section profile/investor_profile.md`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, name, err := e.readMarkdownUnder(args[0])
			if err != nil {
				return err
			}
			out, err := pickSections(string(b), name, args[1:], summary, headings)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(e.out, out)
			return err
		},
	}
	c.Flags().BoolVar(&summary, "summary", false, "print the ## Summary section")
	c.Flags().StringArrayVar(&headings, "heading", nil, "print each ##, ### or #### heading starting with this prefix, with its block (repeatable)")
	return c
}

// readMarkdownUnder reads a .md file by its path under data/ (never outside it, never data/secrets);
// a bare file name missing from data/ is looked up in the newest run folder.
func (e *env) readMarkdownUnder(rel string) ([]byte, string, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("%s: give a path under the data folder", rel)
	}
	if strings.Split(filepath.ToSlash(clean), "/")[0] == "secrets" {
		return nil, "", fmt.Errorf("%s: data/secrets is never read", rel)
	}
	if !strings.HasSuffix(strings.ToLower(clean), ".md") {
		return nil, "", fmt.Errorf("%s: pb run section reads Markdown (.md) files", rel)
	}
	b, err := os.ReadFile(e.path(clean))
	if err == nil {
		return b, clean, nil
	}
	if !os.IsNotExist(err) {
		return nil, "", err
	}
	if !strings.ContainsAny(rel, `/\`) {
		alt := filepath.Join("runs", e.latestRun(), clean)
		if b, err := os.ReadFile(e.path(alt)); err == nil {
			return b, alt, nil
		}
		return nil, "", fmt.Errorf("%s: no such file under %s (nor %s)", rel, e.data, alt)
	}
	return nil, "", fmt.Errorf("%s: no such file under %s", rel, e.data)
}

// mdHeading is one "##" or "###" heading of a file: its level, text and line.
type mdHeading struct {
	Level int
	Text  string
	Line  int
}

// mdHeadings finds the "##"/"###" (and, for block ends, "#" and deeper) headings outside code fences.
func mdHeadings(lines []string) []mdHeading {
	var out []mdHeading
	fence := false
	for i, l := range lines {
		t := strings.TrimRight(l, "\r")
		if strings.HasPrefix(strings.TrimSpace(t), "```") {
			fence = !fence
			continue
		}
		if fence || !strings.HasPrefix(t, "#") {
			continue
		}
		n := len(t) - len(strings.TrimLeft(t, "#"))
		if n > 6 || len(t) == n || t[n] != ' ' {
			continue
		}
		out = append(out, mdHeading{n, strings.TrimSpace(t[n:]), i})
	}
	return out
}

// headingMatches says whether a heading's text starts with the prefix; a prefix ending in a digit
// must end at a space, a dot or the end of the text there (6.1 is not 6.10).
func headingMatches(text, prefix string) bool {
	if prefix == "" || len(text) < len(prefix) || !strings.EqualFold(text[:len(prefix)], prefix) {
		return false
	}
	last := prefix[len(prefix)-1]
	if last < '0' || last > '9' || len(text) == len(prefix) {
		return true
	}
	next := text[len(prefix)]
	return next == ' ' || next == '.' || next == ')' || next == ':'
}

// isJoinedHead says whether a heading is one pb run join demoted from a batch file: "<text> (<part>.md)".
func isJoinedHead(text, want string) bool {
	i := strings.LastIndex(text, " (")
	return i > 0 && strings.HasSuffix(text, ".md)") && strings.EqualFold(text[:i], want)
}

// pickSections prints the blocks asked for, in file order for headings and in the order asked for
// tickers; with no selector, the headings themselves.
func pickSections(md, name string, tickers []string, summary bool, prefixes []string) (string, error) {
	lines := strings.Split(md, "\n")
	hs := mdHeadings(lines)
	block := func(k int) (int, int) {
		end := len(lines)
		for _, h := range hs[k+1:] {
			if h.Level <= hs[k].Level {
				end = h.Line
				break
			}
		}
		return hs[k].Line, end
	}
	if len(tickers) == 0 && !summary && len(prefixes) == 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s: headings (line: heading)\n", name)
		for _, h := range hs {
			if h.Level == 2 || h.Level == 3 {
				fmt.Fprintf(&b, "%d: %s %s\n", h.Line+1, strings.Repeat("#", h.Level), h.Text)
			}
		}
		return b.String(), nil
	}
	var picked [][2]int
	add := func(k int) {
		lo, hi := block(k)
		for _, p := range picked {
			if lo >= p[0] && hi <= p[1] {
				return // already inside a block printed
			}
		}
		picked = append(picked, [2]int{lo, hi})
	}
	if summary {
		found := false
		for k, h := range hs {
			// A joined file (pb run join) keeps each batch's summary as "#### Summary (<part>.md)".
			if h.Level == 2 && strings.EqualFold(h.Text, "Summary") || h.Level == 4 && isJoinedHead(h.Text, "Summary") {
				add(k)
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("%s: no \"## Summary\" section", name)
		}
	}
	if len(prefixes) > 0 {
		var sel []int
		for k, h := range hs {
			if h.Level < 2 || h.Level > 4 {
				continue
			}
			for _, p := range prefixes {
				if headingMatches(h.Text, strings.TrimSpace(p)) {
					sel = append(sel, k)
					break
				}
			}
		}
		if len(sel) == 0 {
			return "", fmt.Errorf("%s: no ##, ### or #### heading starts with %s", name, strings.Join(prefixes, ", "))
		}
		// Wider blocks first, so a sub-heading inside a picked section is not printed twice.
		sort.SliceStable(sel, func(i, j int) bool { return hs[sel[i]].Level < hs[sel[j]].Level })
		for _, k := range sel {
			add(k)
		}
		sort.SliceStable(picked, func(i, j int) bool { return picked[i][0] < picked[j][0] })
	}
	if len(tickers) > 0 {
		at := map[string]int{}
		var found []string
		for k, h := range hs {
			if h.Level != 2 || strings.EqualFold(h.Text, "Summary") {
				continue
			}
			t := headTicker(h.Text)
			if _, dup := at[t]; !dup {
				at[t] = k
				if isTickerHead(h.Text, nil) {
					found = append(found, t)
				}
			}
		}
		var miss []string
		for _, t := range tickers {
			if _, ok := at[strings.ToUpper(strings.TrimSpace(t))]; !ok {
				miss = append(miss, t)
			}
		}
		if len(miss) > 0 {
			return "", fmt.Errorf("%s: no section for %s (sections found: %s)", name, strings.Join(miss, ", "), strings.Join(found, ", "))
		}
		for _, t := range tickers {
			add(at[strings.ToUpper(strings.TrimSpace(t))])
		}
	}
	var b strings.Builder
	for _, p := range picked {
		blk := strings.TrimRight(strings.Join(lines[p[0]:p[1]], "\n"), "\n ")
		blk = strings.TrimRight(strings.TrimSuffix(blk, "---"), "\n ") // the rule between sections
		b.WriteString(blk)
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}
