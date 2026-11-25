package core

// plural_test.go - the sibilant and the final-y cases, which are the whole
// reason this is not an inline "%ss".

import "testing"

// TestPluralCountsSibilantsWithoutStuttering: "address" takes "es", and the
// naive suffix that produced "4 more addresss" must not come back.
func TestPluralCountsSibilantsWithoutStuttering(t *testing.T) {
	for _, c := range []struct {
		n    int
		noun string
		want string
	}{
		{1, "cause", "1 cause"},
		{2, "cause", "2 causes"},
		{1, "more address", "1 more address"},
		{4, "more address", "4 more addresses"},
		{0, "gap", "0 gaps"},
		{2, "batch", "2 batches"},
		{2, "dish", "2 dishes"},
		{2, "box", "2 boxes"},
		{1, "company", "1 company"},
		{2, "company", "2 companies"},
		{2, "day", "2 days"},
		{2, "journey", "2 journeys"},
	} {
		if got := Plural(c.n, c.noun); got != c.want {
			t.Errorf("Plural(%d, %q) = %q, want %q", c.n, c.noun, got, c.want)
		}
	}
}
