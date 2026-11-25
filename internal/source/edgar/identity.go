package edgar

// identity.go - whether two spellings name one company: the broker's truncated
// upper-case description against the name the SEC holds.

import (
	"regexp"
	"strings"
)

// legalWords carry no identity: the legal form, the share class and what the
// broker appends to a line.
var legalWords = map[string]bool{}

func init() {
	for w := range strings.FieldsSeq("INC INCORPORATED CORP CORPORATION CO COS COMPANY COMPANIES LTD LIMITED LLC LP " +
		"PLC SA SE NV AG AB ASA OYJ SPA AKTIENGESELLSCHAFT ADR ADS CL CLASS THE SHS SHARES NON VTG REG REGISTERED " +
		"SPONSORED SP HOLDING HOLDINGS GROUP GRP NY A B C N V CI PUBL") {
		legalWords[w] = true
	}
}

var (
	stateSuffix = regexp.MustCompile(`/[A-Z]{2,3}/?\s*$`)
	notAlnum    = regexp.MustCompile(`[^A-Z0-9]+`)
)

// nameWords is a name reduced to the words that identify the company.
func nameWords(name string) []string {
	s := stateSuffix.ReplaceAllString(strings.ToUpper(strings.TrimSpace(name)), "")
	s = strings.NewReplacer(".", "", "'", "").Replace(s)
	var out []string
	for w := range strings.FieldsSeq(notAlnum.ReplaceAllString(s, " ")) {
		if !legalWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// abbreviates reports one word as a shortening of the other, as NATL is of
// NATIONAL and INTERN of INTERNATIONAL: same first letter, the rest in order.
func abbreviates(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	if a == "" || a[0] != b[0] {
		return false
	}
	rest := b
	for i := range len(a) {
		at := strings.IndexByte(rest, a[i])
		if at < 0 {
			return false
		}
		rest = rest[at+1:]
	}
	return true
}

// SameCompany compares two names word for word. The count must match, as PRUDENTIAL
// PLC is not PRUDENTIAL FINANCIAL; a name with no identifying words matches nothing.
func SameCompany(a, b string) bool {
	x, y := nameWords(a), nameWords(b)
	if len(x) == 0 || len(x) != len(y) {
		return false
	}
	for i := range x {
		if !abbreviates(x[i], y[i]) {
			return false
		}
	}
	return true
}
