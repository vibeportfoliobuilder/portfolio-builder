package edgar

// proxy.go - the DEF 14A: the pay-ratio trio and the board's independent count
// as figures, plus four windows carried whole. The pay table is never parsed.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Proxy is what this package reads out of one DEF 14A.
type Proxy struct {
	// BoardSize and IndependentDirectors are as the proxy states them, both zero
	// unless ONE sentence stated both: pairing two sentences builds a wrong ratio.
	BoardSize            int
	IndependentDirectors int
	// IndependenceStatement is the sentence the two counts were read out of,
	// verbatim.
	IndependenceStatement string

	// ChairIsCEO is set only when the proxy attaches the combined title to the
	// person this program already believes is the CEO - see readChair.
	ChairIsCEO     bool
	ChairStatement string

	// The Item 402(u) trio, in whole dollars as the proxy states them.
	CEOTotalPay       int64
	MedianEmployeePay int64
	PayRatioToMedian  float64
	PayRatioStatement string

	// Excerpts are the windows of the proxy carried across whole.
	Excerpts []Excerpt

	// Notes name what could not be read and why, in the dossier's own gap
	// sentence vocabulary. A proxy that stated everything produces none.
	Notes []string
}

// Excerpt is one window of a proxy, carried verbatim.
type Excerpt struct {
	// Name is the stable key a reader (and a prompt) refers to it by.
	Name string
	// Title is the human name of the section.
	Title string
	// Heading is the line the window starts at, bounded - see firstLine.
	Heading string
	// Text is the window itself, verbatim.
	Text string
	// Chars is len([]rune(Text)).
	Chars int
	// Method says how this occurrence of the heading was chosen out of the
	// several the document carries.
	Method string
	// Truncated reports whether the window ran to its character budget rather
	// than to the end of the section.
	Truncated bool
}

// excerptSpec is one window this package looks for.
type excerptSpec struct {
	name    string
	title   string
	anchor  *regexp.Regexp
	markers []string
	chars   int
}

// proxyExcerpts are the four windows, with character budgets measured on five real
// proxies as enough to carry each section whole.
var proxyExcerpts = []excerptSpec{
	{
		name:    "summary_compensation_table",
		title:   "Summary Compensation Table",
		anchor:  regexp.MustCompile(`(?im)^\s*(?:\d{4}\s+)?summary compensation table`),
		markers: []string{"salary", "total", "compensation"},
		chars:   9000,
	},
	{
		name:    "compensation_discussion",
		title:   "Compensation Discussion and Analysis",
		anchor:  regexp.MustCompile(`(?im)^\s*compensation discussion (?:and|&) analysis`),
		markers: []string{"performance", "incentive", "compensation"},
		chars:   14000,
	},
	{
		name:    "pay_ratio",
		title:   "CEO Pay Ratio",
		anchor:  regexp.MustCompile(`(?im)^\s*(?:ceo\s+)?pay ratio`),
		markers: []string{"median", "compensation"},
		chars:   8000,
	},
	{
		name:    "board_independence",
		title:   "Director Independence",
		anchor:  regexp.MustCompile(`(?im)^\s*(?:(?:director|board)\s+independence\b|independence\s+(?:of|assessment|determination)\b)`),
		markers: []string{"independent", "director", "board"},
		chars:   5000,
	},
}

// ReadProxy extracts the figures and windows from a DEF 14A's text. An empty
// ceoName costs the chair question and nothing else.
func ReadProxy(text, ceoName string) Proxy {
	var p Proxy
	// The counts first: their sentence is the fallback anchor for the independence
	// excerpt. Patterns run on a flowed copy, whose offsets still index the original.
	flow := strings.ReplaceAll(text, "\n", " ")
	independenceAt := p.readIndependence(flow)
	for _, spec := range proxyExcerpts {
		if e, ok := excerptFor(text, spec); ok {
			p.Excerpts = append(p.Excerpts, e)
			continue
		}
		if spec.name == independenceExcerpt && independenceAt >= 0 {
			p.Excerpts = append(p.Excerpts, windowAt(text, spec, independenceAt))
			continue
		}
		p.Notes = append(p.Notes, fmt.Sprintf(
			"edgar: the proxy's %s section could not be located by the headings this decoder looks for, so it is not excerpted; the filing itself carries it",
			spec.title))
	}
	p.readChair(flow, ceoName)
	p.readPayRatio(ceoName)
	return p
}

// independenceExcerpt names the one window with a second way of being found.
const independenceExcerpt = "board_independence"

// windowAt is the excerpt taken around a position rather than under a heading,
// starting a little before the sentence so the window does not open mid-clause.
func windowAt(text string, spec excerptSpec, at int) Excerpt {
	start := max(at-windowLead, 0)
	end := min(start+spec.chars, len(text))
	body := strings.TrimSpace(text[start:end])
	return Excerpt{
		Name:      spec.name,
		Title:     spec.title,
		Heading:   firstLine(text[start:]),
		Text:      body,
		Chars:     len([]rune(body)),
		Method:    "no heading matched; window taken around the sentence the independence counts were read out of",
		Truncated: end < len(text) && end == start+spec.chars,
	}
}

// windowLead is how much context a positional window opens with.
const windowLead = 500

// Excerpt returns one window by name. The pointer receiver is for recvcheck's
// consistent-receiver rule, not mutation.
func (p *Proxy) Excerpt(name string) (Excerpt, bool) {
	for _, e := range p.Excerpts {
		if e.Name == name {
			return e, true
		}
	}
	return Excerpt{}, false
}

// excerptFor takes the window under the best occurrence of a heading, scored by
// how many expected words the text under it carries, latest winning a tie.
func excerptFor(text string, spec excerptSpec) (Excerpt, bool) {
	locs := spec.anchor.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return Excerpt{}, false
	}
	bestScore, bestAt, bestIdx := -1, 0, 0
	for i, loc := range locs {
		win := strings.ToLower(text[loc[0]:min(loc[0]+spec.chars, len(text))])
		score := 0
		for _, m := range spec.markers {
			if strings.Contains(win, m) {
				score++
			}
		}
		// A table of contents carries every marker word a real section does; what tells
		// them apart is the page numbers beside each line.
		if looksLikeContents(text[loc[0]:min(loc[0]+contentsProbeChars, len(text))]) {
			score -= len(spec.markers)
		}
		// >= rather than >: a later occurrence with the same score wins, the earlier
		// ones being the contents and the cross-references.
		if score >= bestScore {
			bestScore, bestAt, bestIdx = score, loc[0], i
		}
	}
	if bestScore == 0 {
		return Excerpt{}, false
	}
	end := min(bestAt+spec.chars, len(text))
	body := strings.TrimSpace(text[bestAt:end])
	return Excerpt{
		Name:    spec.name,
		Title:   spec.title,
		Heading: firstLine(text[bestAt:]),
		Text:    body,
		Chars:   len([]rune(body)),
		Method: fmt.Sprintf("heading occurrence %d of %d, matching %d of %d expected words (%s)",
			bestIdx+1, len(locs), bestScore, len(spec.markers), strings.Join(spec.markers, ", ")),
		Truncated: end < len(text) && end == bestAt+spec.chars,
	}, true
}

// contentsProbeChars is how much of a candidate window is examined for the
// page-number shape of a table of contents.
const contentsProbeChars = 1500

// contentsPageNumbers is how many bare-number lines make a window a contents
// block: four is past coincidence and under the twenty a real one carries.
const contentsPageNumbers = 4

var (
	bareNumberLine = regexp.MustCompile(`(?m)^\s*\d{1,3}\s*$`)
	groupedNumber  = regexp.MustCompile(`\d,\d\d\d`)
)

// contentsGroupedNumbers keeps a real financial table from being read as a
// contents block: only the table has money written with thousands separators.
const contentsGroupedNumbers = 3

// looksLikeContents reports whether a window is a table of contents rather than
// the section it names. See the call site.
func looksLikeContents(win string) bool {
	if len(groupedNumber.FindAllStringIndex(win, contentsGroupedNumbers)) >= contentsGroupedNumbers {
		return false
	}
	return len(bareNumberLine.FindAllStringIndex(win, contentsPageNumbers)) >= contentsPageNumbers
}

// independenceREs are the independence sentence shapes in the order tried; each yields a
// count and a size in that order unless sizeFirst says otherwise.
var independenceREs = []struct {
	re        *regexp.Regexp
	sizeFirst bool
}{
	// "11 of 12 director nominees are independent", and the heading-style "10 of 12
	// Director nominees independent" - the verb is optional, a summary box having none.
	{re: regexp.MustCompile(`(?i)\b(\d{1,2}|` + numberWordsAlt + `)\s+of\s+(?:our\s+|the\s+)?(\d{1,2}|` + numberWordsAlt + `)\s+(?:current\s+)?(?:board\s+|director\s+)?(?:nominees|directors|director nominees|board members|members)\b[^.\n]{0,40}?\bindependent\b`)},
	// "Of our 12 Board nominees, 11 are independent"
	{re: regexp.MustCompile(`(?i)\bof\s+(?:our|the)\s+(\d{1,2}|` + numberWordsAlt + `)\s+(?:current\s+)?(?:board\s+|director\s+)?(?:nominees|directors|director nominees|board members|members)\b[^.\n]{0,20}?,?\s*(\d{1,2}|` + numberWordsAlt + `)\s+(?:are|were)\s+independent\b`), sizeFirst: true},
}

// numberWordsAlt is the spelled-out alternation the patterns above accept,
// because a proxy writes "eleven of twelve" as readily as "11 of 12".
const numberWordsAlt = `one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty`

var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7,
	"eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13,
	"fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18,
	"nineteen": 19, "twenty": 20,
}

// maxBoardSize is the sanity ceiling: a US public company board is 5 to 15
// directors, so a match above 30 is two unrelated numbers in one sentence.
const maxBoardSize = 30

// readIndependence fills the two counts, or records why it could not. A sentence
// must state BOTH numbers: a count without its denominator invites arithmetic.
func (p *Proxy) readIndependence(text string) int {
	for _, cand := range independenceREs {
		for _, loc := range cand.re.FindAllStringSubmatchIndex(text, -1) {
			m := []string{text[loc[0]:loc[1]], text[loc[2]:loc[3]], text[loc[4]:loc[5]]}
			a, b := parseCount(m[1]), parseCount(m[2])
			indep, size := a, b
			if cand.sizeFirst {
				indep, size = b, a
			}
			if indep <= 0 || size <= 0 || indep > size || size > maxBoardSize {
				continue
			}
			p.IndependentDirectors, p.BoardSize = indep, size
			p.IndependenceStatement = firstLine(m[0])
			return loc[0]
		}
	}
	p.Notes = append(p.Notes, "edgar: the proxy states no sentence naming both the board size and how many of its members are independent in a shape this decoder reads "+
		"(the two shapes it reads are \"N of M directors are independent\" and \"of our M nominees, N are independent\") - "+
		"the board_independence excerpt carries the section's own words instead")
	return -1
}

func parseCount(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, ok := numberWords[s]; ok {
		return n
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// readChair answers "does the CEO also chair the board", and only where the
// combined title is attached to this CEO's own name - Amazon's proxy says it a dozen times.
func (p *Proxy) readChair(text, ceoName string) {
	surname := surnameOf(ceoName)
	if surname == "" {
		p.Notes = append(p.Notes, "edgar: whether the CEO also chairs the board was not read from the proxy - it is only read when this program already knows the CEO's name, and no source stated one for this ticker")
		return
	}
	for _, re := range chairPatterns(surname) {
		if m := re.FindStringSubmatch(text); m != nil {
			p.ChairIsCEO = true
			p.ChairStatement = firstLine(m[0])
			return
		}
	}
}

// chairPatterns is the combined-role phrase bound to one name, in the two orders
// a proxy writes it. Adjacent, or Apple's director table reports its own CEO as chair.
func chairPatterns(surname string) []*regexp.Regexp {
	n := regexp.QuoteMeta(surname)
	return []*regexp.Regexp{
		regexp.MustCompile(`(?i)` + n + `[^.\n]{0,` + chairGap + `}?` + chairRole),
		regexp.MustCompile(`(?i)` + chairRole + `[^.\n]{0,` + chairGap + `}?` + n + `\b`),
	}
}

// chairRole is the combined title itself, in either order of the two offices.
const chairRole = `(?:chair(?:man|woman|person)?(?:\s+of\s+the\s+board)?\s*(?:,\s*)?(?:and|&)\s+(?:our\s+|its\s+|the\s+)?(?:chief executive officer|ceo)|(?:chief executive officer|ceo)\s*(?:,\s*)?(?:and|&)\s+(?:our\s+|its\s+|the\s+)?chair(?:man|woman|person)?(?:\s+of\s+the\s+board)?)`

// chairGap is how much may sit between the name and the title: a clause - ",
// our" or ", who serves as" - and no more.
const chairGap = "24"

// surnameOf takes the family name out of either spelling this program holds a
// CEO under: "Andrew R. Jassy" and a broker's own "Jassy, Andrew".
func surnameOf(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if i := strings.IndexByte(name, ','); i > 0 {
		return strings.TrimSpace(name[:i])
	}
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	// A generational suffix is not a surname.
	switch strings.ToLower(strings.Trim(last, ".")) {
	case "jr", "sr", "ii", "iii", "iv":
		if len(fields) > 1 {
			return fields[len(fields)-2]
		}
	}
	return last
}

// The Item 402(u) sentence shapes. All three figures are optional independently:
// a proxy stating the ratio with the amounts three pages earlier is common.
var (
	// "...the ratio ... was 480 to 1", "...is 533 to 1", "...was 129:1".
	ratioRE = regexp.MustCompile(`(?i)ratio[^.\n]{0,220}?\b(\d[\d,]*(?:\.\d+)?)\s*(?:to|:|-to-|-to\s|\sto-)\s*1\b`)
	// Amazon writes the same ratio backwards. Read second, and only where the first
	// found nothing: "1 to 480" is not a spelling anybody uses.
	ratioReversedRE = regexp.MustCompile(`(?i)ratio[^.\n]{0,220}?\b1\s*(?:to|:|-to-|-to\s|\sto-)\s*(\d[\d,]*(?:\.\d+)?)\b`)

	ceoPayREs = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:annual\s+)?total compensation of (?:our|the) (?:ceo|chief executive officer)[^.\n]{0,80}?\$\s?(\d[\d,]*)`),
		regexp.MustCompile(`(?i)(?:our|the) (?:ceo|chief executive officer)['’]?s?[^.\n]{0,80}?(?:annual\s+)?total compensation[^.\n]{0,60}?\$\s?(\d[\d,]*)`),
	}
	medianPayREs = []*regexp.Regexp{
		regexp.MustCompile(`(?i)median[^.\n]{0,60}?employee[^.\n]{0,120}?\$\s?(\d[\d,]*)`),
		regexp.MustCompile(`(?i)median[^.\n]{0,120}?compensation[^.\n]{0,80}?\$\s?(\d[\d,]*)`),
	}
)

// ceoPayPatterns is the fixed shapes plus, where the CEO's name is known, the one
// that needs it: two of five proxies never write "our CEO" in the disclosure.
func ceoPayPatterns(ceoName string) []*regexp.Regexp {
	out := ceoPayREs
	if s := surnameOf(ceoName); s != "" {
		out = append(append([]*regexp.Regexp{}, out...), regexp.MustCompile(
			`(?i)`+regexp.QuoteMeta(s)+`['’]?s?[^.\n]{0,40}?total compensation[^.\n]{0,160}?\$\s?(\d[\d,]*)`))
	}
	return out
}

// The sanity bands, past which a figure is refused rather than written onto a
// dossier an agent then reasons from.
const (
	minCEOPay    = 1e5
	maxCEOPay    = 1e9
	minMedianPay = 1e3
	maxMedianPay = 5e6
	maxPayRatio  = 1e5
)

// readPayRatio reads the trio out of the pay-ratio window, scoped there because
// "$", "to 1" and "median" all occur throughout a proxy.
func (p *Proxy) readPayRatio(ceoName string) {
	e, ok := p.Excerpt("pay_ratio")
	if !ok {
		p.Notes = append(p.Notes, "edgar: the proxy's CEO pay ratio disclosure could not be located by heading, so the CEO's stated total compensation, the median employee's and the ratio between them are all unfilled")
		return
	}
	win := strings.ReplaceAll(e.Text, "\n", " ")
	if m := ratioRE.FindStringSubmatch(win); m != nil {
		p.setRatio(parseMoney(m[1]), firstLine(m[0]))
	} else if m := ratioReversedRE.FindStringSubmatch(win); m != nil {
		p.setRatio(parseMoney(m[1]), firstLine(m[0]))
	}
	for _, re := range ceoPayPatterns(ceoName) {
		m := re.FindStringSubmatch(win)
		if m == nil {
			continue
		}
		// The disclosure states both figures in one sentence, so a match whose own text
		// names the median is refused: on Apple's the two differ by five hundred.
		if strings.Contains(strings.ToLower(m[0]), "median") {
			continue
		}
		if v := parseMoney(m[1]); v >= minCEOPay && v <= maxCEOPay {
			p.CEOTotalPay = int64(v)
			break
		}
	}
	for _, re := range medianPayREs {
		if m := re.FindStringSubmatch(win); m != nil {
			if v := parseMoney(m[1]); v >= minMedianPay && v <= maxMedianPay {
				p.MedianEmployeePay = int64(v)
				break
			}
		}
	}
	p.checkRatio()
}

func (p *Proxy) setRatio(v float64, sentence string) {
	if v <= 0 || v > maxPayRatio {
		return
	}
	p.PayRatioToMedian = v
	p.PayRatioStatement = sentence
}

// checkRatio names it when the two figures do not divide to the stated ratio.
// All three are still recorded: the disagreement means one came from the wrong sentence.
func (p *Proxy) checkRatio() {
	if p.CEOTotalPay == 0 || p.MedianEmployeePay == 0 || p.PayRatioToMedian == 0 {
		return
	}
	got := float64(p.CEOTotalPay) / float64(p.MedianEmployeePay)
	if diff := got - p.PayRatioToMedian; diff > ratioTolerance*p.PayRatioToMedian || -diff > ratioTolerance*p.PayRatioToMedian {
		p.Notes = append(p.Notes, fmt.Sprintf(
			"edgar: the proxy's stated pay ratio (%.0f to 1) does not match the two figures read beside it ($%d against $%d, which is %.0f to 1) - "+
				"one of the three was read out of the wrong sentence; all three are recorded as read and none is derived",
			p.PayRatioToMedian, p.CEOTotalPay, p.MedianEmployeePay, got))
	}
}

// ratioTolerance is how far the computed ratio may sit from the stated one: a
// proxy rounds what it prints, so a few percent is agreement and a factor is not.
const ratioTolerance = 0.05

// parseMoney reads "96,496,790" as a number. It is used for the ratio too,
// which is written with the same separators.
func parseMoney(s string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	if err != nil {
		return 0
	}
	return v
}
