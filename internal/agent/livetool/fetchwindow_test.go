package livetool

// fetchwindow_test.go - a proxy's pay sections sat past the 24,000-character
// clip; the clipped text now names where they start, and find/offset/max read them.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchReadsPastTheClip(t *testing.T) {
	dir := t.TempDir()
	doc := "Table of contents Summary Compensation Table " + strings.Repeat("filler ", 6000) +
		"Summary Compensation Table Name Salary Bonus CEO 1,500,000 " + strings.Repeat("more ", 2000) + "Pay Ratio 300 to 1"
	srv := stubDoc(t, doc)
	input := filepath.Join(dir, "edgar.yaml")
	if err := os.WriteFile(input, []byte(srv.URL+"/def14a"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := NewFetcher(nil, dir, []string{input}, day("2031-03-09"))
	out, err := f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/def14a"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[truncated:") || !strings.Contains(out, "Summary Compensation Table @ ") || !strings.Contains(out, "Pay Ratio @ ") {
		t.Fatalf("no section index:\n%s", out[len(out)-300:])
	}
	// The section itself is the last match, past the table of contents.
	out, err = f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/def14a", Find: "pay ratio", Max: 100})
	if err != nil || !strings.Contains(out, "[from character ") || !strings.Contains(out, "Pay Ratio 300 to 1") {
		t.Fatalf("find: %v\n%s", err, out)
	}
	out, err = f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/def14a", Offset: 42050, Max: 60})
	if err != nil || !strings.Contains(out, "[from character 42050") {
		t.Fatalf("offset: %v\n%s", err, out)
	}
	if _, err := f.Fetch(context.Background(), FetchArgs{URL: srv.URL + "/def14a", Find: "no such words"}); err == nil {
		t.Fatal("a missing find text must be an error, not the start of the document")
	}
}
