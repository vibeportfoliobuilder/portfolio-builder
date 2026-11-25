package livetool

// fetch_test.go - provenance is the whole tool: an address this run was never
// given is refused, and everything it does open is recorded before it answers.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const filingHTML = `<html><head><title>Item 1A Risk Factors</title>
<style>p{color:red}</style></head>
<body><h1>Risks</h1><p>Competition &amp; pricing pressure.</p></body></html>`

func stubDoc(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnAddressThisRunWasNeverGivenIsRefused(t *testing.T) {
	dir := t.TempDir()
	f := NewFetcher(nil, dir, nil, day("2026-09-24"))

	_, err := f.Fetch(context.Background(), FetchArgs{URL: "https://invented.example/page"})
	if !errors.Is(err, ErrNoProvenance) {
		t.Fatalf("want ErrNoProvenance, got %v", err)
	}
}

// file:// would read the disk through a tool whose purpose is reaching outside
// the snapshot, so the scheme is refused before provenance is even considered.
func TestOnlyHTTPAndHTTPSAreOpened(t *testing.T) {
	dir := t.TempDir()
	f := NewFetcher(nil, dir, nil, day("2026-09-24"))
	for _, u := range []string{"file:///etc/passwd", "ftp://host/x", "gopher://host"} {
		if _, err := f.Fetch(context.Background(), FetchArgs{URL: u}); !errors.Is(err, ErrNotWeb) {
			t.Errorf("%s: want ErrNotWeb, got %v", u, err)
		}
	}
}

func TestAURLPrintedInAnInputIsProvenance(t *testing.T) {
	dir := t.TempDir()
	srv := stubDoc(t, filingHTML)

	input := filepath.Join(dir, "filing_sections.yaml")
	body := "sections:\n- name: Item 1A\n  truncated: true\n  url: " + srv.URL + "/10k\n"
	if err := os.WriteFile(input, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f := NewFetcher(nil, dir, []string{input}, day("2026-09-24"))

	out, err := f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/10k"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(out, "Competition & pricing pressure.") {
		t.Errorf("the document text did not come back: %q", out)
	}
	if strings.Contains(out, "<p>") || strings.Contains(out, "color:red") {
		t.Errorf("markup and style survived into the answer: %q", out)
	}
}

func TestAURLAnEarlierSearchRecordedIsProvenance(t *testing.T) {
	dir := t.TempDir()
	srv := stubDoc(t, filingHTML)
	if _, err := Append(dir, day("2026-09-24"), []Source{{URL: srv.URL + "/found"}}); err != nil {
		t.Fatal(err)
	}
	f := NewFetcher(nil, dir, nil, day("2026-09-24"))

	if _, err := f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/found"}); err != nil {
		t.Fatalf("a URL web_search already recorded must be fetchable: %v", err)
	}
}

func TestWhatWasFetchedIsRecordedBeforeItIsReturned(t *testing.T) {
	dir := t.TempDir()
	srv := stubDoc(t, filingHTML)
	input := filepath.Join(dir, "thesis.md")
	if err := os.WriteFile(input, []byte("see "+srv.URL+"/doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := NewFetcher(nil, dir, []string{input}, day("2026-09-24"))

	if _, err := f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/doc"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	l, err := ReadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got *Source
	for i := range l.Sources {
		if l.Sources[i].URL == srv.URL+"/doc" {
			got = &l.Sources[i]
		}
	}
	if got == nil {
		t.Fatalf("the fetched document was not recorded: %+v", l.Sources)
	}
	if got.Title != "Item 1A Risk Factors" {
		t.Errorf("the document's title was not recorded: %q", got.Title)
	}
	if got.RetrievedAt != "2026-09-24" {
		t.Errorf("retrieved_at is not the threaded instant: %q", got.RetrievedAt)
	}
}

func TestALongDocumentSaysItWasTruncated(t *testing.T) {
	dir := t.TempDir()
	srv := stubDoc(t, strings.Repeat("word ", bodyLimit))
	input := filepath.Join(dir, "thesis.md")
	if err := os.WriteFile(input, []byte(srv.URL+"/long"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := NewFetcher(nil, dir, []string{input}, day("2026-09-24"))

	out, err := f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/long"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(out, "[truncated:") {
		t.Error("a truncated document must say so, or the agent reads a fragment as the whole")
	}
}

func TestReadablePassesPlainTextThrough(t *testing.T) {
	if got := readable([]byte("  just words  ")); got != "just words" {
		t.Fatalf("plain text should pass through trimmed: %q", got)
	}
}
