package edgar

// filings.go - the submissions document. EDGAR publishes the index COLUMN-WISE,
// zipped into rows here because a mismatched index is a date on the wrong form.

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/docfmt"
)

// The form types this package looks for. An amended annual report is recognised
// as one, with its form string carried verbatim.
const (
	Form10K    = "10-K"
	Form10Q    = "10-Q"
	FormDEF14A = "DEF 14A"
	Form8K     = "8-K"
)

// Submissions is the part of the SEC's document this package reads.
type Submissions struct {
	CIK      int      `json:"-"`
	Name     string   `json:"name"`
	Tickers  []string `json:"tickers"`
	SICDesc  string   `json:"sicDescription"`
	FYEnd    string   `json:"fiscalYearEnd"`
	Category string   `json:"category"`
	Filings  struct {
		Recent recentFilings `json:"recent"`
	} `json:"filings"`
}

// recentFilings is EDGAR's column-wise block, decoded verbatim.
type recentFilings struct {
	AccessionNumber []string `json:"accessionNumber"`
	FilingDate      []string `json:"filingDate"`
	ReportDate      []string `json:"reportDate"`
	Form            []string `json:"form"`
	Items           []string `json:"items"`
	PrimaryDocument []string `json:"primaryDocument"`
	PrimaryDocDesc  []string `json:"primaryDocDescription"`
}

// Filing is one row of that block, with the two addresses a reader wants.
type Filing struct {
	Form string
	// Filed is when the document reached the SEC; Period is what it covers, empty
	// for a form that covers none - an 8-K reports an event.
	Filed  time.Time
	Period time.Time
	// Accession is the filing's id, in the dashed spelling EDGAR prints.
	Accession string
	// DocURL addresses the primary document itself - the 10-K, the proxy - and
	// IndexURL addresses the filing's index page, which lists every exhibit.
	DocURL   string
	IndexURL string
	// Items are the 8-K item numbers, verbatim ("2.02,9.01"). Empty for every
	// other form.
	Items string
	// Desc is EDGAR's own description of the primary document, which is where
	// a proxy says whether it is the annual meeting's.
	Desc string
}

// Rows zips the column-wise block into filings, newest first. A short column
// truncates the set: nothing asserts the arrays are parallel.
func (s Submissions) Rows() []Filing {
	r := s.Filings.Recent
	n := min(len(r.Form), len(r.FilingDate), len(r.AccessionNumber))
	out := make([]Filing, 0, n)
	for i := range n {
		f := Filing{
			Form:      strings.TrimSpace(r.Form[i]),
			Accession: strings.TrimSpace(r.AccessionNumber[i]),
			Items:     docfmt.Cell(r.Items, i),
			Desc:      docfmt.Cell(r.PrimaryDocDesc, i),
		}
		if f.Form == "" || f.Accession == "" {
			continue
		}
		var ok bool
		if f.Filed, ok = docfmt.ParseISODay(r.FilingDate[i]); !ok {
			continue // a filing with no filed date is not a fact about a date
		}
		f.Period, _ = docfmt.ParseISODay(docfmt.Cell(r.ReportDate, i))
		f.DocURL, f.IndexURL = filingURLs(s.CIK, f.Accession, docfmt.Cell(r.PrimaryDocument, i))
		out = append(out, f)
	}
	// Newest first: the document arrives that way today and nothing says it
	// must, and every accessor below takes "the latest" as the first match.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Filed.After(out[j].Filed) })
	return out
}

// filingURLs builds the two addresses: EDGAR's archive path uses the accession
// number twice, undashed as the directory and dashed in the index file's name.
func filingURLs(cik int, accession, primaryDoc string) (doc, index string) {
	bare := strings.ReplaceAll(accession, "-", "")
	dir := formatCIK(cik) + "/" + bare
	index = WWWBase + "/Archives/edgar/data/" + dir + "/" + accession + "-index.htm"
	if primaryDoc == "" {
		return index, index
	}
	return WWWBase + "/Archives/edgar/data/" + dir + "/" + primaryDoc, index
}

// formatCIK is the CIK as the archive path spells it: decimal, UNPADDED. Every
// other route pads it to ten digits, and the padded form answers 404 here.
func formatCIK(cik int) string {
	return strconv.Itoa(cik)
}

// Latest returns the newest filing of a form, an amendment counting as the same
// form: naming the superseded one would point at the corrected version.
func (s Submissions) Latest(form string) (Filing, bool) {
	for _, f := range s.Rows() {
		if sameForm(f.Form, form) {
			return f, true
		}
	}
	return Filing{}, false
}

// Recent returns up to n filings of a form, newest first.
func (s Submissions) Recent(form string, n int) []Filing {
	var out []Filing
	for _, f := range s.Rows() {
		if sameForm(f.Form, form) {
			out = append(out, f)
			if len(out) == n {
				break
			}
		}
	}
	return out
}

// FiledFor is the filing that covers a period end. A five-day window, the two
// sides rounding differently, and a 10-K is preferred over a 10-Q for one period.
func (s Submissions) FiledFor(period time.Time) (Filing, bool) {
	if period.IsZero() {
		return Filing{}, false
	}
	var best Filing
	var found bool
	for _, f := range s.Rows() {
		if !sameForm(f.Form, Form10K) && !sameForm(f.Form, Form10Q) {
			continue
		}
		if f.Period.IsZero() || core.DaysBetween(f.Period, period) > periodMatchDays {
			continue
		}
		switch {
		case !found:
			best, found = f, true
		case sameForm(f.Form, Form10K) && !sameForm(best.Form, Form10K):
			best = f
		}
	}
	return best, found
}

// periodMatchDays is how far a filing's stated period may sit from the
// statement's own period end and still be the same period. See FiledFor.
const periodMatchDays = 5

// sameForm compares a filing's form with the one wanted, counting an amendment
// as the same form. EDGAR writes the suffix as "/A".
func sameForm(got, want string) bool {
	got = strings.ToUpper(strings.TrimSpace(got))
	want = strings.ToUpper(strings.TrimSpace(want))
	return got == want || strings.HasPrefix(got, want+"/")
}
