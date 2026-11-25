package docfmt

import (
	"encoding/csv"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
)

// csv.go - reading a table out of a file that is only approximately a CSV.
// Nothing here knows what a fund or an issuer is.

// Small CSV/number/date utilities (tolerant, stdlib-only).

type Header struct {
	Cols []string // lower-cased, trimmed
}

func (h Header) Index(name string) int {
	name = strings.ToLower(name)
	for i, c := range h.Cols {
		if c == name {
			return i
		}
	}
	return -1
}

func (h Header) IndexAny(names ...string) int {
	for _, n := range names {
		if i := h.Index(n); i >= 0 {
			return i
		}
	}
	// then substring match
	for _, n := range names {
		if i := h.IndexContains(n); i >= 0 {
			return i
		}
	}
	return -1
}

// IndexLabel finds the column whose header IS one of these names, allowing only
// a trailer that is plainly not part of the word: "price (usd)" but not "average price paid".
func (h Header) IndexLabel(names ...string) int {
	for _, n := range names {
		if i := h.Index(n); i >= 0 {
			return i
		}
	}
	for _, n := range names {
		n = strings.ToLower(n)
		for i, c := range h.Cols {
			if len(c) > len(n) && strings.HasPrefix(c, n) && !isWordByte(c[len(n)]) {
				return i
			}
		}
	}
	return -1
}

// isWordByte reports whether a byte continues a word, so "price (usd)" matches
// the label "price" and "prices" does not. h.Cols is already lower-cased.
func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

func (h Header) IndexContains(sub string) int {
	sub = strings.ToLower(sub)
	for i, c := range h.Cols {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

// ReadCSVFindHeader returns ONE table: the rows under the row that looks MOST
// like a header - SSGA's caption also does - ending where a second such row begins.
func ReadCSVFindHeader(b []byte, mustHave, wantAny []string) ([][]string, Header, error) {
	all, err := ReadCSVRows(b)
	if err != nil && len(all) == 0 {
		return nil, Header{}, err
	}
	hdrIdx, best := -1, -1
	for i, row := range all {
		low := LowerRow(row)
		if !LooksLikeHeaderRow(low) {
			continue
		}
		if !RowHas(low, mustHave) || (wantAny != nil && !RowHas(low, wantAny)) {
			continue
		}
		if n := countNamedColumns(low, mustHave, wantAny); n > best {
			hdrIdx, best = i, n
		}
	}
	if hdrIdx < 0 {
		return nil, Header{}, errors.New("header row not found")
	}
	rows := all[hdrIdx+1:]
	if n := nextTable(rows, mustHave, wantAny, best); n >= 0 {
		rows = rows[:n]
	}
	return rows, Header{Cols: LowerRow(all[hdrIdx])}, nil
}

// nextTable is where a SECOND table of the same kind starts, or -1. iShares
// publishes some funds twice in one file, under a header of its own.
func nextTable(rows [][]string, mustHave, wantAny []string, best int) int {
	for i, row := range rows {
		low := LowerRow(row)
		if !LooksLikeHeaderRow(low) || !RowHas(low, mustHave) {
			continue
		}
		if wantAny != nil && !RowHas(low, wantAny) {
			continue
		}
		// As many named columns as the header that won: a data row naming five
		// columns is not a shape any publisher emits, and a weaker match is a row.
		if countNamedColumns(low, mustHave, wantAny) >= best {
			return i
		}
	}
	return -1
}

// countNamedColumns is how many of a row's cells read as a column name, one per
// cell however many words it holds: the question is how many columns it names.
func countNamedColumns(low []string, sets ...[]string) int {
	n := 0
	for _, c := range low {
		if c == "" {
			continue
		}
		if cellNamesAColumn(c, sets) {
			n++
		}
	}
	return n
}

func cellNamesAColumn(cell string, sets [][]string) bool {
	for _, needles := range sets {
		for _, needle := range needles {
			if strings.Contains(cell, strings.ToLower(needle)) {
				return true
			}
		}
	}
	return false
}

// ReadCSVRows reads every row, tolerating the ragged and half-quoted files issuers publish and
// one leading byte-order mark. A partial parse is returned rather than discarded.
func ReadCSVRows(b []byte) ([][]string, error) {
	rd := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(b), "\ufeff")))
	rd.Comma = DetectDelimiter(b)
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	return rd.ReadAll()
}

func LowerRow(r []string) []string {
	out := make([]string, len(r))
	for i, c := range r {
		out[i] = strings.ToLower(strings.TrimSpace(c))
	}
	return out
}

func RowHas(low []string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	for _, n := range needles {
		n = strings.ToLower(n)
		for _, c := range low {
			if strings.Contains(c, n) {
				return true
			}
		}
	}
	return false
}

func DetectDelimiter(b []byte) rune {
	// Sample the first non-empty line.
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[:i]
	}
	if strings.Count(s, ";") > strings.Count(s, ",") {
		return ';'
	}
	return ','
}

func CleanCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"")
	return strings.TrimSpace(s)
}

// ParseNum tolerates %, thousands separators, both decimal marks, parentheses negatives and
// a leading currency symbol; NaN, an infinity and an exponent past float64's range are no number.
func ParseNum(s string) (float64, bool) {
	s = CleanCell(s)
	if s == "" || s == "-" || strings.EqualFold(s, "n/a") {
		return 0, false
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = s[1 : len(s)-1]
	}
	s = strings.TrimSuffix(s, "%")

	// Scientific notation, before the cleanup below: a converted sheet renders net
	// assets as 4.15044179717E9, and stripping the exponent makes four billion four.
	if v, err := strconv.ParseFloat(s, 64); err == nil || errors.Is(err, strconv.ErrRange) {
		// The cleanup would read "1e400" as 1400, so a value out of range stops here.
		if err != nil || !core.Stated(v) {
			return 0, false
		}
		if neg {
			v = -v
		}
		return v, true
	}

	s = strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r == '.', r == ',', r == '-', r == '+':
			return r
		default:
			return -1
		}
	}, s)

	hasComma, hasDot := strings.Contains(s, ","), strings.Contains(s, ".")
	switch {
	case hasComma && hasDot:
		// Both present: the LAST separator is the decimal mark, the other is a
		// thousands separator. "1,234.56" -> dot decimal; "1.234,56" -> comma.
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case hasComma:
		// Only commas: thousands grouping if they split the string into groups of
		// exactly three digits; otherwise a European decimal mark.
		if isThousandGrouped(s, ",") {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.Replace(s, ",", ".", 1)
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

// isThousandGrouped reports whether sep splits s into a leading group of 1-3
// digits and further groups of exactly three. At least two groups.
func isThousandGrouped(s, sep string) bool {
	s = strings.TrimLeft(s, "+-")
	parts := strings.Split(s, sep)
	if len(parts) < 2 {
		return false
	}
	if len(parts[0]) < 1 || len(parts[0]) > 3 {
		return false
	}
	for _, p := range parts[1:] {
		if len(p) != 3 {
			return false
		}
	}
	return true
}

func ParseDate(s string) (time.Time, bool) {
	s = CleanCell(s)
	if t, ok := ParseDateLayouts(s); ok {
		return t, true
	}
	// Retried rather than tried first, so every layout below keeps behaving
	// exactly as it did: this only ever runs on a cell nothing else could read.
	if alt := NormaliseSept(s); alt != s {
		return ParseDateLayouts(alt)
	}
	return time.Time{}, false
}

// ParseISODay is the strict reader: a trimmed yyyy-mm-dd as its UTC midnight,
// false for a blank or any other shape, where ParseDate tries every layout.
func ParseISODay(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// septRe matches the one month abbreviation that is not three letters: Go takes
// "Sep" or "September", and BlackRock writes "12/Sept/2025" - 4,200 dropped rows.
var septRe = regexp.MustCompile(`(?i)\bsept\b`)

func NormaliseSept(s string) string { return septRe.ReplaceAllString(s, "Sep") }

// germanMonths is what BlackRock's "(DE)" workbook calls the months, Austrian
// "Jän" included. Only names: a numeric month is never rewritten from here.
var germanMonths = map[string]string{
	"jan": "Jan", "jän": "Jan", "jaen": "Jan", "januar": "Jan",
	"feb": "Feb", "februar": "Feb",
	"mär": "Mar", "märz": "Mar", "maerz": "Mar", "mar": "Mar",
	"apr": "Apr", "april": "Apr",
	"mai": "May",
	"jun": "Jun", "juni": "Jun",
	"jul": "Jul", "juli": "Jul",
	"aug": "Aug", "august": "Aug",
	"sep": "Sep", "sept": "Sep", "september": "Sep",
	"okt": "Oct", "oktober": "Oct", "oct": "Oct",
	"nov": "Nov", "november": "Nov",
	"dez": "Dec", "dezember": "Dec", "dec": "Dec",
}

// germanDateRe matches a date whose month is a NAME: "04.Sep.2026", and
// "15.Juli2026", which carries no separator at all before the year.
var germanDateRe = regexp.MustCompile(`^(\d{1,2})\.?\s*([\p{L}]+)\.?\s*(\d{4})$`)

// NormaliseGermanDate rewrites such a date as dd.Mmm.yyyy, and returns s
// unchanged when the middle token is not a month name - so nothing else moves.
func NormaliseGermanDate(s string) string {
	m := germanDateRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return s
	}
	mon, ok := germanMonths[strings.ToLower(m[2])]
	if !ok {
		return s
	}
	return m[1] + "." + mon + "." + m[3]
}

func ParseDateLayouts(s string) (time.Time, bool) {
	for _, layout := range []string{
		"2006-01-02", "02/01/2006", "01/02/2006", "02.01.2006",
		"2 Jan 2006", "02 Jan 2006", "Jan 2, 2006", "2006/01/02",
		"20060102", "02-Jan-2006", "January 2, 2006",
		// BlackRock slashes a three-letter month: 05/Aug/2026. Without this every
		// iShares export's date column silently fails to parse.
		"02/Jan/2006", "2/Jan/2006",
		// The "(DE)" workbook dots them instead: 04.Sep.2026, via NormaliseGermanDate.
		"02.Jan.2006", "2.Jan.2006",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// LooksLikeHeaderRow reports whether a row could be a header: at least two
// filled cells, none long enough to be prose.
func LooksLikeHeaderRow(low []string) bool {
	filled := 0
	for _, c := range low {
		if c == "" {
			continue
		}
		if len(c) > 48 {
			return false
		}
		filled++
	}
	return filled >= 2
}

// FirstBytes is the head of a payload, for an error that has to say what
// arrived: a consent page and an empty body fail parsing the same way.
func FirstBytes(b []byte) string {
	if len(b) > 16 {
		b = b[:16]
	}
	return string(b)
}
