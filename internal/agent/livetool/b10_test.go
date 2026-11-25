package livetool

// b10_test.go - fetch_url's text: the inline-XBRL header dropped, a PDF never returned as bytes,
// and the text kept beside the ledger.

import (
	"bytes"
	"compress/zlib"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadableDropsTheInlineXBRLHeader(t *testing.T) {
	page := `<html><head><title>8-K</title></head><body><div style="display:none"><ix:header><ix:hidden>dei:EntityCentralIndexKey 0001705696 false FY</ix:hidden></ix:header></div><p>Item 5.02 Departure of Directors. John M. Sullivan was appointed.</p></body></html>`
	got := readable([]byte(page))
	if strings.Contains(got, "EntityCentralIndexKey") || !strings.Contains(got, "John M. Sullivan was appointed") {
		t.Errorf("%q", got)
	}
}

func TestAPDFIsTextOrANoteNeverBytes(t *testing.T) {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write([]byte("BT /F1 12 Tf (Example World Equity High Income UCITS ETF. Distribution yield 10.45% as of 05 Nov 2026. Total expense ratio 0.35%. Share class currency USD. Use of income distributing, monthly. Fund base currency USD. Benchmark MSCI World.) Tj ET"))
	_ = w.Close()
	pdf := append([]byte("%PDF-1.7\n1 0 obj << /Filter /FlateDecode >>\nstream\n"), z.Bytes()...)
	pdf = append(pdf, []byte("\nendstream\nendobj\n%%EOF")...)
	if got := readable(pdf); !strings.Contains(got, "Distribution yield 10.45%") {
		t.Errorf("%q", got)
	}
	junk := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{0x01, 0xfe, 0x80}, 400)...)
	if got := readable(junk); !strings.HasPrefix(got, "[PDF document") {
		t.Errorf("binary came back: %q", got[:min(60, len(got))])
	}
}

func TestSaveTextKeepsWhatTheReaderSaw(t *testing.T) {
	dir := t.TempDir()
	rel, err := saveText(dir, "https://www.sec.gov/x.htm", "the body")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil || strings.TrimSpace(string(b)) != "the body" || !strings.HasPrefix(rel, "sources/") {
		t.Errorf("%s %q %v", rel, b, err)
	}
}
