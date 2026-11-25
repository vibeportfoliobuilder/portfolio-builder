package edgar

// sections.go - Item 1 and Item 1A out of a 10-K. Each is named twice, in the
// contents and in the body, so the rule is the WIDEST SPAN: 25-100 chars against 41-92k.

import (
	"fmt"
	"regexp"
	"strings"
)

// minSectionChars is the shortest run this package will call a section: 2,000,
// far below the smallest real one and far above a contents-block pairing.
const minSectionChars = 2000

// Section is one extracted run of a filing's text.
type Section struct {
	// Item is the section as the form names it: "1" or "1A".
	Item string
	// Title is this package's own name for it, for a reader of the stored
	// file: "Business", "Risk Factors".
	Title string
	// Heading is the line the extraction started at, verbatim. It is the
	// evidence that the boundary was found where a reader would have found it.
	Heading string
	// EndedAt is the heading the section was cut at - the next item's - or
	// "end of document" where nothing followed it.
	EndedAt string
	// Text is the section, verbatim, from its heading to the next item's.
	Text string
	// Chars is len([]rune(Text)).
	Chars int
}

// Found reports whether the section was located at all.
func (s Section) Found() bool { return s.Text != "" }

// Annual is what this package reads out of one 10-K.
type Annual struct {
	Business    Section
	RiskFactors Section
	// Notes name every boundary this could not find and why, in the sentence
	// vocabulary the dossier's own gap lines use.
	Notes []string
}

// The heading patterns, each anchored to the start of a line and tolerating the
// four separators seen in practice plus a line break after the item number.
var (
	item1RE  = regexp.MustCompile(`(?im)^\s*item\s*1\s*[.:)\-–—]?\s*\n?\s*business\b`)
	item1ARE = regexp.MustCompile(`(?im)^\s*item\s*1a\s*[.:)\-–—]?\s*\n?\s*risk\s*factors\b`)
	item1BRE = regexp.MustCompile(`(?im)^\s*item\s*1b\s*[.:)\-–—]?\s*\n?\s*unresolved\b`)
	item2RE  = regexp.MustCompile(`(?im)^\s*item\s*2\s*[.:)\-–—]?\s*\n?\s*propert`)
)

// ReadAnnual extracts Item 1 and Item 1A from a 10-K's text. A form that is not
// one, or whose headings are not found, returns nothing found and a note per section.
func ReadAnnual(text string) Annual {
	var a Annual
	a.Business = extract(text, "1", "Business", item1RE, []*regexp.Regexp{item1ARE, item1BRE, item2RE})
	a.RiskFactors = extract(text, "1A", "Risk Factors", item1ARE, []*regexp.Regexp{item1BRE, item2RE})
	for _, s := range []struct {
		sec  Section
		item string
	}{{a.Business, "Item 1 (Business)"}, {a.RiskFactors, "Item 1A (Risk Factors)"}} {
		if !s.sec.Found() {
			a.Notes = append(a.Notes, fmt.Sprintf(
				"edgar: %s could not be located in the 10-K's text - no heading pair spanning at least %d characters was found, so the section is not extracted rather than extracted wrongly",
				s.item, minSectionChars))
		}
	}
	return a
}

// extract is the widest-span rule: ends is tried in order and the EARLIEST end
// after a start wins, some filers omitting Item 1B entirely.
func extract(text, item, title string, start *regexp.Regexp, ends []*regexp.Regexp) Section {
	starts := start.FindAllStringIndex(text, -1)
	if len(starts) == 0 {
		return Section{}
	}
	// Two passes, and the order is the safety: a candidate with no following item
	// heading runs to the end of the document and would beat the real section.
	if best := widest(text, item, title, starts, ends, true); best.Found() {
		return best
	}
	return widest(text, item, title, starts, ends, false)
}

// widest is extract's one pass: the longest span among the candidates,
// optionally restricted to those that end at a real heading.
func widest(text, item, title string, starts [][]int, ends []*regexp.Regexp, needEnd bool) Section {
	best := Section{}
	for _, s := range starts {
		e, found := earliestEnd(text, s[1], ends)
		if needEnd && !found {
			continue
		}
		if e-s[0] < minSectionChars {
			continue
		}
		body := strings.TrimSpace(text[s[0]:e])
		if len([]rune(body)) <= len([]rune(best.Text)) {
			continue
		}
		best = Section{
			Item:    item,
			Title:   title,
			Heading: firstLine(text[s[0]:s[1]]),
			EndedAt: endHeading(text, e),
			Text:    body,
		}
	}
	best.Chars = len([]rune(best.Text))
	return best
}

// earliestEnd is the first end heading at or after from. Where none was found
// the span runs to the end, and the caller decides whether that is allowed.
func earliestEnd(text string, from int, ends []*regexp.Regexp) (int, bool) {
	out, found := len(text), false
	for _, re := range ends {
		if loc := re.FindStringIndex(text[from:]); loc != nil && from+loc[0] < out {
			out, found = from+loc[0], true
		}
	}
	return out, found
}

// endHeading names what the section was cut at, for the stored record.
func endHeading(text string, at int) string {
	if at >= len(text) {
		return "end of document"
	}
	return firstLine(text[at:])
}

// headingLabelChars bounds what is stored as a heading: Apple opens Item 1A's
// first paragraph on the heading's own line, which is four thousand characters.
const headingLabelChars = 120

// firstLine is the heading as one bounded line, with the line breaks a printer
// puts inside it folded to spaces: "Item 1A.\nRisk Factors" is one heading.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " " + strings.TrimSpace(s[i+1:])
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > headingLabelChars {
		s = strings.TrimSpace(string(r[:headingLabelChars])) + "..."
	}
	return s
}
