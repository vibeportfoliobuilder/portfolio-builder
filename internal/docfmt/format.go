// Package docfmt is the canonical rendering of a value into a committed byte, so
// that writing one record twice on two machines produces identical bytes.
package docfmt

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
)

// FormatNum renders a float as the shortest decimal that parses back exactly,
// never in exponent form. A non-finite value is "" and negative zero is "0".
func FormatNum(v float64) string {
	if !core.Stated(v) {
		return ""
	}
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// FormatDate renders a calendar date as YYYY-MM-DD, the zero time as "". The
// .UTC() is a guard: local midnight east of Greenwich formats a day early.
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// FormatInstant renders an instant as RFC 3339 in UTC to the second, the form every
// stored or served stamp takes; the zero time prints as 0001-01-01T00:00:00Z.
func FormatInstant(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// CSVField renders one cell, quoting only when the cell would otherwise be
// misread - a comma, a quote, a newline, or edge whitespace an editor would eat.
func CSVField(s string) string {
	need := strings.ContainsAny(s, ",\"\r\n")
	if !need && s != "" {
		if isSpaceByte(s[0]) || isSpaceByte(s[len(s)-1]) {
			need = true
		}
	}
	if !need {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// isSpaceByte reports the ASCII whitespace an editor or spreadsheet is likely to
// trim. Byte-wise, and no UTF-8 continuation byte is in the set.
func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\v' || b == '\f'
}

// WriteCSVRows writes a header and its rows as one CSV document: LF, no BOM,
// trailing newline. A ragged row is an error rather than a padded line.
func WriteCSVRows(w io.Writer, header []string, rows [][]string) error {
	if len(header) == 0 {
		return fmt.Errorf("csv: refusing to write a document with no header")
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		for i, c := range cells {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(CSVField(c))
		}
		b.WriteByte('\n')
	}

	writeRow(header)
	for i, r := range rows {
		if len(r) != len(header) {
			return fmt.Errorf("csv: row %d has %d cells, header has %d", i, len(r), len(header))
		}
		writeRow(r)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// LooksLikeHTML reports whether a payload is a web page rather than data - an
// error page or cookie wall at http 200. Only the first 512 bytes are read.
func LooksLikeHTML(b []byte) bool {
	head := strings.ToLower(strings.TrimSpace(string(b[:min(len(b), 512)])))
	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}
