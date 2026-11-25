package livetool

// search_test.go - the ordering property: the ledger is written BEFORE the
// answer, and a ledger that cannot be written is a failed call, not a quiet one.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/tavily"
)

func stubSearcher(t *testing.T, dir, body string) *Searcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	t.Setenv(tavily.KeyEnv, "test-key")
	c, err := tavily.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.HTTP, c.Base = srv.Client(), srv.URL
	return NewSearcher(c, dir, day("2026-09-24"))
}

const twoHits = `{"query":"q","results":[
  {"url":"https://a.example/1","title":"One","content":"first body","score":0.9},
  {"url":"https://b.example/2","title":"Two","content":"second body","score":0.8}]}`

func TestSearchRecordsEveryResultItReturns(t *testing.T) {
	dir := t.TempDir()
	s := stubSearcher(t, dir, twoHits)

	out, err := s.Search(context.Background(), SearchArgs{Query: "why does it matter"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	l, err := ReadLedger(dir)
	if err != nil {
		t.Fatalf("ReadLedger: %v", err)
	}
	if len(l.Sources) != 2 {
		t.Fatalf("want both results recorded, got %d", len(l.Sources))
	}
	for _, want := range []string{"https://a.example/1", "https://b.example/2"} {
		if !strings.Contains(out, want) {
			t.Errorf("the answer does not carry %s, so it cannot be cited", want)
		}
	}
	if l.Sources[0].Queries == nil || l.Sources[0].Queries[0] != "why does it matter" {
		t.Errorf("the query was not recorded beside the citation: %+v", l.Sources[0])
	}
	if l.Sources[0].RetrievedAt != "2026-09-24" {
		t.Errorf("retrieved_at is not the threaded instant: %q", l.Sources[0].RetrievedAt)
	}
}

// If the ledger cannot be written the call fails. Returning results anyway
// would hand the model citations no reader could check.
func TestAnUnwritableLedgerFailsTheCall(t *testing.T) {
	dir := t.TempDir()
	s := stubSearcher(t, dir, twoHits)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot make the directory read-only here: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	out, err := s.Search(context.Background(), SearchArgs{Query: "q"})
	if err == nil {
		t.Fatalf("want an error when the ledger cannot be written, got %q", out)
	}
	if out != "" {
		t.Errorf("a failed call must return no results, got %q", out)
	}
}

func TestNoResultsIsAnAnswerNotAnError(t *testing.T) {
	dir := t.TempDir()
	s := stubSearcher(t, dir, `{"query":"q","results":[]}`)

	out, err := s.Search(context.Background(), SearchArgs{Query: "nothing at all"})
	if err != nil {
		t.Fatalf("an empty result set must not be an error: %v", err)
	}
	if !strings.Contains(out, "No results") {
		t.Errorf("the answer should say nothing was found, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, LedgerFile)); !os.IsNotExist(err) {
		t.Error("an empty search should write no ledger file")
	}
}

func TestClipCutsOnARuneBoundary(t *testing.T) {
	if got := clip(strings.Repeat("é", 10), 4); got != "éééé..." {
		t.Fatalf("clip cut a multi-byte rune: %q", got)
	}
	if got := clip("  short  ", 100); got != "short" {
		t.Fatalf("clip should trim and pass short strings through: %q", got)
	}
}
