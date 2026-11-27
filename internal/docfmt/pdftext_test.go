package docfmt

// pdftext_test.go - the call deck's page selection on plain text (no pdftotext needed), and the
// extraction itself on a generated one-page PDF when pdftotext is installed.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSplitPDFText(t *testing.T) {
	pages, err := SplitPDFText("Testco Q1 2030\n\n   Results      and   outlook   \n\fsecond page with enough words to count as a text layer for this test deck, which needs a hundred bytes\n\f")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0] != "Testco Q1 2030\nResults  and  outlook" {
		t.Errorf("%q", pages)
	}
	if _, err := SplitPDFText("\f\f\f  \f"); !errors.Is(err, ErrNoTextLayer) {
		t.Errorf("an image-only deck gave %v", err)
	}
}

func TestSelectCallPages(t *testing.T) {
	pages := []string{
		"Testco first quarter 2030", // 1: always kept
		"Agenda and team photos",    // 2: no topic
		"Safe Harbor. Forward-looking statements about guidance, margins and dividends.",                               // 3: legal page
		"Revenue by segment in XTS millions",                                                                           // 4: no topic
		"Gross margin rose 40 bps",                                                                                     // 5: one topic
		"2030 Guidance: revenue up 4%, operating margin 30%",                                                           // 6: guidance first
		"Capital allocation: dividend raised, buyback of XTS 500m, free cash flow XTS 2bn " + strings.Repeat("x", 300), // 7: four topics, long
	}
	ct := SelectCallPages(pages, 10000)
	if !slices.Equal(ct.Kept, []int{1, 5, 6, 7}) || !slices.Equal(ct.NoTopic, []int{2, 4}) || !slices.Equal(ct.Boilerplate, []int{3}) || len(ct.OverCap) != 0 {
		t.Errorf("%+v", ct)
	}
	if !strings.HasPrefix(ct.Text, "[p.1]\nTestco first quarter 2030\n[p.5]\n") {
		t.Errorf("text %q", ct.Text)
	}
	// A tight cap keeps page 1, then guidance (score 1+2), then the four-topic page if it fits.
	ct = SelectCallPages(pages, 120)
	if !slices.Equal(ct.Kept, []int{1, 5, 6}) || !slices.Equal(ct.OverCap, []int{7}) {
		t.Errorf("tight cap: %+v", ct)
	}
	r := ct.Render("earnings_call.pdf")
	for _, want := range []string{"pages 1,5-6 of 7", "p.2,4 (no guidance", "p.3 (safe-harbor", "p.7 (over the text cap)"} {
		if !strings.Contains(r, want) {
			t.Errorf("render lacks %q:\n%s", want, r)
		}
	}
	// Page 1 alone over the cap is clipped, and said so.
	ct = SelectCallPages([]string{strings.Repeat("é", 100)}, 51)
	if !ct.Clipped || len(ct.Text) > 52 || !strings.Contains(ct.Render("x.pdf"), "the end of p.1") {
		t.Errorf("clip: %+v", ct)
	}
}

func TestPageList(t *testing.T) {
	if got := pageList([]int{1, 2, 3, 5, 7, 8}); got != "1-3,5,7-8" {
		t.Error(got)
	}
}

// onePagePDF writes a minimal one-page PDF whose text is s (ASCII, no parentheses).
func onePagePDF(t *testing.T, s string) string {
	t.Helper()
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", s)
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	p := filepath.Join(t.TempDir(), "deck.pdf")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEarningsCallTextWithPDFToText(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	text := "Testco 2030 outlook: dividend up 5 percent, free cash flow of XTS 2 billion and an operating margin near 30 percent"
	ct, err := EarningsCallText(context.Background(), onePagePDF(t, text), 0)
	if err != nil {
		t.Fatal(err)
	}
	if ct.Pages != 1 || !strings.Contains(ct.Text, "[p.1]\nTestco 2030 outlook") {
		t.Errorf("%+v", ct)
	}
	if _, err := EarningsCallText(context.Background(), onePagePDF(t, " "), 0); !errors.Is(err, ErrNoTextLayer) {
		t.Errorf("an empty page gave %v", err)
	}
}

func TestEarningsCallTextWithoutPDFToText(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := EarningsCallText(context.Background(), "deck.pdf", 0); !errors.Is(err, ErrNoPDFToText) {
		t.Errorf("got %v", err)
	}
}
