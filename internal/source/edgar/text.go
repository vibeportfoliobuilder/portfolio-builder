package edgar

// text.go - a filing document as readable text: a block tag becomes a newline
// and every other tag is removed leaving NOTHING, or "BUSINESS" arrives as "B USINESS".

import (
	"html"
	"regexp"
	"strings"
)

// blockTags are the tags whose presence means a line break. Every other tag is
// removed leaving nothing at all, or a styled "BUSINESS" reads "B USINESS".
const blockTags = `p|div|br|tr|td|th|table|thead|tbody|tfoot|h[1-6]|li|ul|ol|section|article|hr|caption|dt|dd`

var (
	// RE2 has no backreference, so the two paired tags are two patterns rather
	// than one with a \1.
	scriptRE  = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	styleRE   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	commentRE = regexp.MustCompile(`(?s)<!--.*?-->`)
	blockRE   = regexp.MustCompile(`(?is)</?(?:` + blockTags + `)(?:\s[^>]*)?/?>`)
	tagRE     = regexp.MustCompile(`(?s)<[^>]*>`)
	hspaceRE  = regexp.MustCompile(`[ \t\r\f\v]+`)
	paddedNL  = regexp.MustCompile(` *\n *`)
	blankRE   = regexp.MustCompile(`\n{2,}`)
)

// invisible is the encoding-level normalisation: the two non-space spaces a
// printer emits, and the zero-width space, which no whitespace rule would catch.
var invisible = strings.NewReplacer(
	"\u00a0", " ", // no-break space
	"\u2007", " ", // figure space
	"\u200b", "", // zero-width space
	"\ufeff", "", // byte-order mark, seen mid-document in printer output
)

// Text renders one filing document as plain text, line-oriented: one heading,
// paragraph or table cell per line, with no blank lines.
func Text(doc []byte) string {
	s := string(doc)
	s = scriptRE.ReplaceAllString(s, " ")
	s = styleRE.ReplaceAllString(s, " ")
	s = commentRE.ReplaceAllString(s, " ")
	s = blockRE.ReplaceAllString(s, "\n")
	s = tagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = invisible.Replace(s)
	s = hspaceRE.ReplaceAllString(s, " ")
	s = paddedNL.ReplaceAllString(s, "\n")
	s = blankRE.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}
