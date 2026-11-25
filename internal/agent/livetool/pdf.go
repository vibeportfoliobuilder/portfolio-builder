package livetool

// pdf.go - the text of a fetched PDF (an issuer fact sheet), which came back as raw bytes (fix
// list B10). No PDF library is vendored: the content streams are inflated and the strings shown
// by Tj/TJ read out. Fonts with custom encodings defeat that; then the reader is told so instead of
// being handed binary.

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

var (
	pdfStream = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	pdfTj     = regexp.MustCompile(`(?s)\((.*?[^\\])\)\s*Tj|\[(.*?)\]\s*TJ`)
	pdfParen  = regexp.MustCompile(`(?s)\((.*?[^\\])\)`)
)

// pdfText reads what text a PDF's streams show, or a one-line note when it cannot.
func pdfText(b []byte) string {
	var out strings.Builder
	for _, m := range pdfStream.FindAllSubmatch(b, -1) {
		data := m[1]
		if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
			if inf, err := io.ReadAll(io.LimitReader(r, 4<<20)); err == nil {
				data = inf
			}
		}
		for _, t := range pdfTj.FindAllSubmatch(data, -1) {
			if len(t[1]) > 0 {
				out.WriteString(pdfUnescape(string(t[1])))
			} else {
				for _, p := range pdfParen.FindAllSubmatch(t[2], -1) {
					out.WriteString(pdfUnescape(string(p[1])))
				}
			}
			out.WriteByte(' ')
		}
		out.WriteByte('\n')
	}
	text := strings.Join(strings.Fields(out.String()), " ")
	if readableShare(text) < 0.8 || len(text) < 200 {
		return fmt.Sprintf("[PDF document, %d KB: its text could not be extracted here (embedded font encoding). Open it in a browser, or fetch the issuer's HTML page instead.]", len(b)/1024)
	}
	return text
}

func pdfUnescape(s string) string {
	r := strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`, `\n`, " ", `\r`, " ", `\t`, " ")
	return r.Replace(s)
}

// readableShare is the share of letters, digits, spaces and common punctuation in s.
func readableShare(s string) float64 {
	if s == "" {
		return 0
	}
	ok, n := 0, 0
	for _, r := range s {
		n++
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || strings.ContainsRune(".,;:%()-/&'+$€£", r) {
			ok++
		}
	}
	return float64(ok) / float64(n)
}
