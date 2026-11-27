package pbcli

// calltext.go - pb calltext: data/stocks/<T>/earnings_call.txt, the page-tagged text of the
// earnings-call deck a connector saved, so an agent reads the pages that matter as text.

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/docfmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// Call-deck file names under data/stocks/<T>/.
const (
	callPDF  = "earnings_call.pdf"
	callText = "earnings_call.txt"
)

// callTextResult is one name's outcome.
type callTextResult struct {
	Ticker  string `json:"ticker"`
	Status  string `json:"status"` // written, no earnings_call.pdf, no text layer, pdftotext not found, error
	File    string `json:"file,omitempty"`
	Pages   int    `json:"pages,omitempty"`
	Kept    string `json:"kept,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	PDFSize int64  `json:"pdf_bytes,omitempty"`
	Note    string `json:"note,omitempty"`
}

func callTextCmd(e *env) *cobra.Command {
	var watch bool
	var maxBytes int
	c := &cobra.Command{
		Use:   "calltext <TICKER ...> | --watchlist",
		Short: "Write earnings_call.txt: the call deck's pages on guidance, dividend, buybacks, FCF and margins as text",
		Long: `Reads data/stocks/<T>/earnings_call.pdf (saved by a broker connector's sync) with pdftotext (poppler)
and writes data/stocks/<T>/earnings_call.txt: page 1 plus the pages that mention guidance or outlook,
the dividend, buybacks or repurchases, capital allocation, free cash flow or margins, each tagged
"[p.N]", the pages naming the most topics first, up to --max bytes; a header lists what was left out.
Cite it as earnings_call.pdf p.N. When the deck is images only ("no text layer", a stale .txt is
removed) or pdftotext is not installed ("pdftotext not found"), nothing is written: read the PDF.
No network.
Examples:
  pb calltext KO MSFT --md
  pb calltext --watchlist`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ts, err := e.targets(args, watch, kindStock)
			if err != nil {
				return err
			}
			var res []callTextResult
			for _, t := range ts {
				res = append(res, e.callText(cmd, t.Ticker, maxBytes))
			}
			if !e.md {
				return e.printJSON(map[string]any{"results": res})
			}
			var b strings.Builder
			for _, r := range res {
				switch r.Status {
				case "written":
					fmt.Fprintf(&b, "- %s: wrote %s (%d bytes; pages %s of %d; the PDF is %d bytes)\n", r.Ticker, r.File, r.Bytes, r.Kept, r.Pages, r.PDFSize)
				default:
					fmt.Fprintf(&b, "- %s: %s", r.Ticker, r.Status)
					if r.Note != "" {
						fmt.Fprintf(&b, ": %s", r.Note)
					}
					b.WriteString("\n")
				}
			}
			_, err = fmt.Fprint(e.out, b.String())
			return err
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "every stock in data/watchlist.yaml")
	c.Flags().IntVar(&maxBytes, "max", docfmt.CallTextMax, "cap on the selected text, in bytes")
	return c
}

// callText writes one name's earnings_call.txt and says what happened.
func (e *env) callText(cmd *cobra.Command, ticker string, maxBytes int) callTextResult {
	r := callTextResult{Ticker: ticker}
	pdf, txt := e.path("stocks", ticker, callPDF), e.path("stocks", ticker, callText)
	st, err := os.Stat(pdf)
	if err != nil {
		r.Status, r.Note = "no "+callPDF, "the call is DATA UNAVAILABLE until a connector's sync saves it"
		return r
	}
	r.PDFSize = st.Size()
	ct, err := docfmt.EarningsCallText(cmd.Context(), pdf, maxBytes)
	switch {
	case errors.Is(err, docfmt.ErrNoPDFToText):
		r.Status, r.Note = "pdftotext not found", "install poppler, or read "+pdf+" as images"
		return r
	case errors.Is(err, docfmt.ErrNoTextLayer):
		r.Status, r.Note = "no text layer", "the deck is images: read "+pdf
		if _, err := os.Stat(txt); err == nil {
			if os.Remove(txt) == nil {
				r.Note += " (removed the stale " + callText + ")"
			}
		}
		return r
	case err != nil:
		r.Status, r.Note = "error", err.Error()
		return r
	}
	body := ct.Render(callPDF)
	if err := store.WriteFileAtomic(txt, []byte(body)); err != nil {
		r.Status, r.Note = "error", err.Error()
		return r
	}
	r.Status, r.File, r.Pages, r.Bytes = "written", txt, ct.Pages, len(body)
	r.Kept = joinInts(ct.Kept)
	return r
}

func joinInts(xs []int) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ",")
}
