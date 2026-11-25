package core

// plural.go - counting a noun in prose, in the kernel because both trees print
// such counts. The naive "%ss" spelling it replaces printed "4 more addresss".

import (
	"fmt"
	"strings"
)

// Plural renders a count with its noun, so a one-bar graft does not say "1
// bars".
func Plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %s", n, pluralOf(noun))
}

// pluralOf is the noun's plural: a consonant before a final y becomes "ies", so
// a count of companies does not read "2 companys".
func pluralOf(noun string) string {
	if n := len(noun); n > 1 && noun[n-1] == 'y' && !isVowel(noun[n-2]) {
		return noun[:n-1] + "ies"
	}
	return noun + pluralSuffix(noun)
}

// isVowel is what a final y turns on: "company" takes "ies" and "day" does not.
func isVowel(c byte) bool { return strings.IndexByte("aeiouAEIOU", c) >= 0 }

// pluralSuffix is "es" after a sibilant and "s" otherwise.
func pluralSuffix(noun string) string {
	for _, end := range []string{"s", "x", "z", "ch", "sh"} {
		if strings.HasSuffix(noun, end) {
			return "es"
		}
	}
	return "s"
}
