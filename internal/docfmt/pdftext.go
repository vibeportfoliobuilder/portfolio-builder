package docfmt

// pdftext.go - an earnings-call deck as page-tagged text: pdftotext (poppler) when it is on
// PATH, then only the pages that speak to guidance, the dividend, buybacks, capital allocation,
// free cash flow or margins (plus page 1), capped, with a note of what was left out.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// CallTextMax is the default cap on the selected text, in bytes.
const CallTextMax = 8000

// noTextLayerBytes is the least text a deck must yield to count as having a text layer.
const noTextLayerBytes = 100

var (
	// ErrNoPDFToText is pdftotext missing from PATH: read the PDF itself.
	ErrNoPDFToText = errors.New("pdftotext not found")
	// ErrNoTextLayer is a deck of images (no extractable text): read the PDF itself.
	ErrNoTextLayer = errors.New("no text layer")
)

// callTopics are what a page must mention to be kept (case-insensitive).
var callTopics = regexp.MustCompile(`(?i)guidance|outlook|dividend|buy-?backs?|repurchas|capital allocation|free cash flow|\bmargins?\b`)

// boilerplate is a legal page (safe harbor, forward-looking statements, non-GAAP definitions): it
// names every topic and says nothing about the quarter, so it is left out (and listed).
var boilerplate = regexp.MustCompile(`(?i)safe harbor|forward-looking statements|cautionary (note|statement)|non-gaap financial measures? (is|are) not`)

var spaceRun = regexp.MustCompile(`[ \t\x{00a0}]{2,}`)

// PDFPageTexts runs pdftotext -layout on a PDF and returns each page's text, runs of spaces
// shortened to two and blank lines dropped (index 0 is page 1). It returns ErrNoPDFToText when
// pdftotext is not on PATH and ErrNoTextLayer when the pages hold (almost) no text.
func PDFPageTexts(ctx context.Context, pdfPath string) ([]string, error) {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, ErrNoPDFToText
	}
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "-layout", "-enc", "UTF-8", pdfPath, "-")
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftotext %s: %v: %s", pdfPath, err, strings.TrimSpace(stderr.String()))
	}
	return SplitPDFText(out.String())
}

// SplitPDFText splits pdftotext output (pages end in a form feed) into cleaned pages, and
// returns ErrNoTextLayer when they hold (almost) no text.
func SplitPDFText(s string) ([]string, error) {
	raw := strings.Split(s, "\f")
	if len(raw) > 1 && strings.TrimSpace(raw[len(raw)-1]) == "" {
		raw = raw[:len(raw)-1]
	}
	pages := make([]string, len(raw))
	total := 0
	for i, p := range raw {
		var lines []string
		for _, l := range strings.Split(p, "\n") {
			if l = strings.TrimSpace(spaceRun.ReplaceAllString(l, "  ")); l != "" {
				lines = append(lines, l)
			}
		}
		pages[i] = strings.Join(lines, "\n")
		total += len(pages[i])
	}
	if total < noTextLayerBytes {
		return nil, ErrNoTextLayer
	}
	return pages, nil
}

// CallText is the selected text of a deck: Kept are the pages in Text (1-based, in page order);
// NoTopic, Boilerplate and OverCap the pages left out for naming no topic, for being a legal page,
// and for the cap.
type CallText struct {
	Pages       int    `yaml:"pages" json:"pages"`
	Kept        []int  `yaml:"kept" json:"kept"`
	NoTopic     []int  `yaml:"no_topic,omitempty" json:"no_topic,omitempty"`
	Boilerplate []int  `yaml:"boilerplate,omitempty" json:"boilerplate,omitempty"`
	OverCap     []int  `yaml:"over_cap,omitempty" json:"over_cap,omitempty"`
	Clipped     bool   `yaml:"clipped,omitempty" json:"clipped,omitempty"` // page 1 alone was over the cap
	Text        string `yaml:"-" json:"-"`
}

// SelectCallPages keeps page 1 and the pages that mention a topic (guidance or outlook, dividend,
// buyback or repurchase, capital allocation, free cash flow, margin) until maxBytes (CallTextMax
// when <= 0): guidance pages and the pages naming the most topics first, a shorter page before a
// longer one. A safe-harbor or non-GAAP legal page is left out. Text tags each page "[p.N]".
func SelectCallPages(pages []string, maxBytes int) CallText {
	if maxBytes <= 0 {
		maxBytes = CallTextMax
	}
	ct := CallText{Pages: len(pages)}
	if len(pages) == 0 {
		return ct
	}
	block := func(i int) string { return "[p." + strconv.Itoa(i+1) + "]\n" + pages[i] + "\n" }
	first := block(0)
	if len(first) > maxBytes {
		first, ct.Clipped = clipUTF8(first, maxBytes)
		first += "\n"
	}
	used := len(first)
	type cand struct{ i, score int }
	var cands []cand
	for i := 1; i < len(pages); i++ {
		hits := map[string]bool{}
		for _, m := range callTopics.FindAllString(pages[i], -1) {
			hits[topicOf(m)] = true
		}
		switch {
		case len(hits) == 0:
			ct.NoTopic = append(ct.NoTopic, i+1)
			continue
		case boilerplate.MatchString(pages[i]):
			ct.Boilerplate = append(ct.Boilerplate, i+1)
			continue
		}
		score := len(hits)
		if hits["guidance"] {
			score += 2 // what the company expects is what the analysts lack most
		}
		cands = append(cands, cand{i, score})
	}
	slices.SortStableFunc(cands, func(a, b cand) int {
		if a.score != b.score {
			return b.score - a.score
		}
		return len(pages[a.i]) - len(pages[b.i])
	})
	keep := map[int]bool{0: true}
	for _, c := range cands {
		if n := len(block(c.i)); used+n <= maxBytes {
			keep[c.i], used = true, used+n
		} else {
			ct.OverCap = append(ct.OverCap, c.i+1)
		}
	}
	slices.Sort(ct.OverCap)
	var b strings.Builder
	for i := range pages {
		if !keep[i] {
			continue
		}
		ct.Kept = append(ct.Kept, i+1)
		if i == 0 {
			b.WriteString(first)
		} else {
			b.WriteString(block(i))
		}
	}
	ct.Text = b.String()
	return ct
}

// topicOf folds a topic match to its topic, so "buyback" and "repurchase" count once.
func topicOf(m string) string {
	m = strings.ToLower(m)
	switch {
	case strings.HasPrefix(m, "guidance"), strings.HasPrefix(m, "outlook"):
		return "guidance"
	case strings.HasPrefix(m, "buy"), strings.HasPrefix(m, "repurchas"):
		return "buyback"
	case strings.HasPrefix(m, "margin"):
		return "margin"
	}
	return m
}

// clipUTF8 cuts s to at most n bytes on a rune boundary.
func clipUTF8(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n], true
}

// Render is the file body: a header naming the source, the pages kept and what was left out,
// then the tagged pages.
func (ct CallText) Render(source string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: pages %s of %d as text (pdftotext); cite as %s p.N. Tables and charts may read out of order: check a figure in the PDF.\n",
		source, pageList(ct.Kept), ct.Pages, source)
	var left []string
	if len(ct.NoTopic) > 0 {
		left = append(left, "p."+pageList(ct.NoTopic)+" (no guidance, dividend, buyback, capital allocation, free cash flow or margin mention)")
	}
	if len(ct.Boilerplate) > 0 {
		left = append(left, "p."+pageList(ct.Boilerplate)+" (safe-harbor or non-GAAP legal text)")
	}
	if len(ct.OverCap) > 0 {
		left = append(left, "p."+pageList(ct.OverCap)+" (over the text cap)")
	}
	if ct.Clipped {
		left = append(left, "the end of p.1 (over the text cap)")
	}
	if len(left) > 0 {
		fmt.Fprintf(&b, "# left out: %s; read them in the PDF.\n", strings.Join(left, "; "))
	}
	b.WriteString(ct.Text)
	return b.String()
}

// pageList writes 1,2,3,5,7,8 as "1-3,5,7-8".
func pageList(ps []int) string {
	var parts []string
	for i := 0; i < len(ps); {
		j := i
		for j+1 < len(ps) && ps[j+1] == ps[j]+1 {
			j++
		}
		if j > i {
			parts = append(parts, fmt.Sprintf("%d-%d", ps[i], ps[j]))
		} else {
			parts = append(parts, strconv.Itoa(ps[i]))
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

// EarningsCallText extracts and selects a call deck's text in one step (maxBytes <= 0: CallTextMax).
// Errors are ErrNoPDFToText and ErrNoTextLayer (read the PDF instead) or pdftotext's own failure.
// A connector calls it after saving the PDF and writes ct.Render(name) next to it.
func EarningsCallText(ctx context.Context, pdfPath string, maxBytes int) (CallText, error) {
	pages, err := PDFPageTexts(ctx, pdfPath)
	if err != nil {
		return CallText{}, err
	}
	return SelectCallPages(pages, maxBytes), nil
}
