package livetool

// ledger_test.go - the two properties the ledger is FOR: it never loses a row,
// and a re-run of the same citations changes no bytes.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

func read(t *testing.T, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, LedgerFile))
	if err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	return b
}

func TestAMissingLedgerIsAnEmptyOne(t *testing.T) {
	l, err := ReadLedger(t.TempDir())
	if err != nil {
		t.Fatalf("a missing file must not be an error: %v", err)
	}
	if len(l.Sources) != 0 {
		t.Fatalf("want no sources, got %d", len(l.Sources))
	}
}

func TestAppendSortsByURLAndDatesWhatItAdds(t *testing.T) {
	dir := t.TempDir()
	added, err := Append(dir, day("2026-09-24"), []Source{
		{URL: "https://b.example/2", Title: "Two"},
		{URL: "https://a.example/1", Title: "One"},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if added != 2 {
		t.Fatalf("want 2 new urls, got %d", added)
	}
	l, err := ReadLedger(dir)
	if err != nil {
		t.Fatalf("ReadLedger: %v", err)
	}
	if got := []string{l.Sources[0].URL, l.Sources[1].URL}; got[0] > got[1] {
		t.Fatalf("rows are not sorted by url: %v", got)
	}
	if l.Sources[0].RetrievedAt != "2026-09-24" {
		t.Fatalf("want the threaded date, got %q", l.Sources[0].RetrievedAt)
	}
}

// The zero-diff property. A later run that finds the same URL must not restate
// its retrieval date, or every re-run of a report commits a diff that means nothing.
func TestASecondAppendOfTheSameURLsIsAZeroDiff(t *testing.T) {
	dir := t.TempDir()
	rows := []Source{
		{URL: "https://a.example/1", Title: "One", Queries: []string{"q1"}},
		{URL: "https://b.example/2", Title: "Two"},
	}
	if _, err := Append(dir, day("2026-09-24"), rows); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	first := read(t, dir)

	added, err := Append(dir, day("2026-11-01"), rows)
	if err != nil {
		t.Fatalf("second Append: %v", err)
	}
	if added != 0 {
		t.Fatalf("want no new urls on a repeat, got %d", added)
	}
	if second := read(t, dir); string(second) != string(first) {
		t.Fatalf("a repeat changed the bytes:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestAppendKeepsEveryURLItAlreadyHad(t *testing.T) {
	dir := t.TempDir()
	if _, err := Append(dir, day("2026-09-24"), []Source{{URL: "https://kept.example/1"}}); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if _, err := Append(dir, day("2026-09-25"), []Source{{URL: "https://new.example/2"}}); err != nil {
		t.Fatalf("second Append: %v", err)
	}
	l, err := ReadLedger(dir)
	if err != nil {
		t.Fatalf("ReadLedger: %v", err)
	}
	if len(l.Sources) != 2 {
		t.Fatalf("want both urls, got %d: %+v", len(l.Sources), l.Sources)
	}
}

func TestASecondSightingFillsBlanksAndUnionsQueries(t *testing.T) {
	dir := t.TempDir()
	const u = "https://a.example/1"
	if _, err := Append(dir, day("2026-09-24"), []Source{
		{URL: u, Title: "One", Queries: []string{"second", "first"}},
	}); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if _, err := Append(dir, day("2026-10-01"), []Source{
		{URL: u, Title: "A different title", Publisher: "Reuters", Queries: []string{"third", "first"}},
	}); err != nil {
		t.Fatalf("second Append: %v", err)
	}
	l, err := ReadLedger(dir)
	if err != nil {
		t.Fatalf("ReadLedger: %v", err)
	}
	got := l.Sources[0]
	if got.Title != "One" {
		t.Errorf("a later sighting overwrote the title: %q", got.Title)
	}
	if got.Publisher != "Reuters" {
		t.Errorf("a blank field was not filled: %q", got.Publisher)
	}
	if got.RetrievedAt != "2026-09-24" {
		t.Errorf("retrieved_at moved: %q", got.RetrievedAt)
	}
	if want := []string{"first", "second", "third"}; len(got.Queries) != 3 ||
		got.Queries[0] != want[0] || got.Queries[1] != want[1] || got.Queries[2] != want[2] {
		t.Errorf("queries are not the sorted union: %v", got.Queries)
	}
}

func TestARowWithNoURLIsDropped(t *testing.T) {
	dir := t.TempDir()
	added, err := Append(dir, day("2026-09-24"), []Source{{URL: "   ", Title: "nowhere"}})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if added != 0 {
		t.Fatalf("want nothing added, got %d", added)
	}
}

func TestTheLedgerIsTwoSpaceWithAFlushDash(t *testing.T) {
	dir := t.TempDir()
	if _, err := Append(dir, day("2026-09-24"), []Source{
		{URL: "https://a.example/1", Title: "One"},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	const want = "sources:\n- url: https://a.example/1\n  title: One\n  retrieved_at: \"2026-09-24\"\n"
	if got := string(read(t, dir)); got != want {
		t.Fatalf("encoding is not the repository's shape:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// A hand-edited file listing one URL twice is deduped rather than refused: the
// guard beside this is about losing a citation, not about tidying one up.
func TestADuplicatedURLIsDedupedNotRefused(t *testing.T) {
	dir := t.TempDir()
	const doc = "sources:\n- url: https://a.example/1\n  title: One\n" +
		"- url: https://a.example/1\n  title: One again\n"
	if err := os.WriteFile(filepath.Join(dir, LedgerFile), []byte(doc), 0o644); err != nil {
		t.Fatalf("seeding the ledger: %v", err)
	}
	if _, err := Append(dir, day("2026-09-24"), nil); err != nil {
		t.Fatalf("Append refused a deduplicable file: %v", err)
	}
	l, err := ReadLedger(dir)
	if err != nil {
		t.Fatalf("ReadLedger: %v", err)
	}
	if len(l.Sources) != 1 {
		t.Fatalf("want the url once, got %d: %+v", len(l.Sources), l.Sources)
	}
}
