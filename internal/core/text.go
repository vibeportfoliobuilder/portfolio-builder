package core

// text.go - string helpers that packages unable to import each other both wrote:
// the first spelling a vendor filled in, and several notes joined as one.

import "strings"

// FirstNonBlank is the first value that is not blank, trimmed; "" when all are.
// A whitespace-only field is a vendor saying nothing, not a value.
func FirstNonBlank(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// NonBlankOr is a when it is not blank, kept as written, and b otherwise: a
// fallback that FirstNonBlank's trim would change.
func NonBlankOr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// RunAdvice is how a message names the database stage that writes what is missing, as a command
// a reader can run, args after it: (run: portfolio-builder database --only "etf fetch").
func RunAdvice(stage string, args ...string) string {
	cmd := `portfolio-builder database --only "` + stage + `"`
	for _, a := range args {
		cmd += " " + a
	}
	return "(run: " + cmd + ")"
}

// JoinNotes joins the notes that say something with "; ": two facts stay two
// sentences, and an empty one leaves no stray separator behind.
func JoinNotes(notes ...string) string {
	var out []string
	for _, n := range notes {
		if n != "" {
			out = append(out, n)
		}
	}
	return strings.Join(out, "; ")
}
