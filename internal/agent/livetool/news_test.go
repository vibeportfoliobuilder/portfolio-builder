package livetool

// news_test.go - the ledger is the provenance that makes reading an unstorable
// body defensible, so the tests are mostly about what Read REFUSES.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFeed is a broker feed with two rows and one readable article, in memory.
type fakeFeed struct{}

const fakeVendor = "testbroker"

func (fakeFeed) Search(context.Context, string, int) ([]NewsRecord, error) {
	return []NewsRecord{
		{Vendor: fakeVendor, ArticleID: "a1", Publisher: "Reuters News", Headline: "Reuters says so", PublishedAt: "2026-09-24T10:22:51Z"},
		{Vendor: fakeVendor, ArticleID: "b2", Publisher: "Benzinga", Headline: "Benzinga says so too", PublishedAt: "2026-09-24T10:20:00Z"},
	}, nil
}

func (fakeFeed) Article(_ context.Context, id string) (string, error) {
	return id + " — Reuters says so [Reuters News]\n\nNot stored; cite the id and publisher.\n\nThe business changed & here is how.", nil
}

func stubNewsroom(t *testing.T, dir string, _ func(string) string) *Newsroom {
	t.Helper()
	return NewNewsroom(fakeFeed{}, "id-1", "RLTY", dir, day("2026-09-24"))
}

func bothRoutes(string) string { return "" }

func TestNewsSearchRecordsEveryItemItLists(t *testing.T) {
	dir := t.TempDir()
	n := stubNewsroom(t, dir, bothRoutes)

	out, err := n.Search(context.Background(), NewsArgs{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	l, err := ReadNewsLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Items) != 2 {
		t.Fatalf("want both items recorded, got %d", len(l.Items))
	}
	if !strings.Contains(out, "a1") || !strings.Contains(out, "b2") {
		t.Error("the answer does not carry the ids the model needs to read an article")
	}
	// Newest first, so a reader sees today's news at the top of the file.
	if l.Items[0].ArticleID != "a1" {
		t.Errorf("ledger is not newest-first: %+v", l.Items)
	}
	if l.Items[0].Vendor != fakeVendor || l.Items[0].Publisher != "Reuters News" {
		t.Errorf("vendor and publisher must stay separate: %+v", l.Items[0])
	}
	if l.Items[0].URL != "" {
		t.Errorf("a broker row with no article URL and must not invent one: %q", l.Items[0].URL)
	}
}

func TestASecondNewsSearchIsAZeroDiff(t *testing.T) {
	dir := t.TempDir()
	n := stubNewsroom(t, dir, bothRoutes)
	if _, err := n.Search(context.Background(), NewsArgs{}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, NewsLedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.Search(context.Background(), NewsArgs{}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dir, NewsLedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("a repeat walk rewrote the archive:\n%s\nvs\n%s", first, second)
	}
}

// Read is the one tool that stores nothing, so the ledger is what makes it
// auditable. An id nobody listed is refused rather than fetched.
func TestReadRefusesAnArticleTheLedgerDoesNotList(t *testing.T) {
	dir := t.TempDir()
	n := stubNewsroom(t, dir, bothRoutes)

	_, err := n.Read(context.Background(), ReadArticleArgs{ArticleID: "never-listed"})
	if !errors.Is(err, ErrArticleNotListed) {
		t.Fatalf("want ErrArticleNotListed, got %v", err)
	}
}

func TestReadStoresNothing(t *testing.T) {
	dir := t.TempDir()
	n := stubNewsroom(t, dir, bothRoutes)
	if _, err := n.Search(context.Background(), NewsArgs{}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, NewsLedgerFile))
	if err != nil {
		t.Fatal(err)
	}

	out, err := n.Read(context.Background(), ReadArticleArgs{ArticleID: "a1"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(out, "The business changed & here is how.") {
		t.Errorf("the article text did not come back: %q", out)
	}
	after, err := os.ReadFile(filepath.Join(dir, NewsLedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("reading an article changed the archive: the body must never be stored")
	}
	// The body must not reach sources.yaml either.
	if _, err := os.Stat(filepath.Join(dir, LedgerFile)); !os.IsNotExist(err) {
		t.Error("reading an article wrote a citation ledger row carrying its text")
	}
}

func TestNewsLedgerKeepsWhatItAlreadyHad(t *testing.T) {
	dir := t.TempDir()
	if _, err := AppendNews(dir, day("2026-09-24"), []NewsRecord{
		{Vendor: fakeVendor, ArticleID: "old", PublishedAt: "2026-09-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendNews(dir, day("2026-09-25"), []NewsRecord{
		{Vendor: fakeVendor, ArticleID: "new", PublishedAt: "2026-09-25T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	l, err := ReadNewsLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Items) != 2 {
		t.Fatalf("want both items, got %d: %+v", len(l.Items), l.Items)
	}
}

// The same story down two pipes is two rows, because the vendor is part of the
// key - and that is exactly why corroboration keys on publisher instead.
func TestTheSameStoryFromTwoVendorsIsTwoRows(t *testing.T) {
	dir := t.TempDir()
	if _, err := AppendNews(dir, day("2026-09-24"), []NewsRecord{
		{Vendor: "broker", ArticleID: "x", Publisher: "Benzinga"},
		{Vendor: "finnhub", ArticleID: "x", Publisher: "Benzinga"},
	}); err != nil {
		t.Fatal(err)
	}
	l, err := ReadNewsLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Items) != 2 {
		t.Fatalf("want a row per pipe, got %d", len(l.Items))
	}
}
