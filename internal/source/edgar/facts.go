package edgar

// facts.go - the XBRL facts document. Not company-CONCEPT: 2026-09-25 it answered
// `units:{"USD":{}}` for STAG's NetIncomeLoss where this document carries 204 values.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/docfmt"
)

// ErrNoXBRLFacts means this filer has published no tagged figures at all, which
// is the answer for a company that files on paper or under another regulator.
var ErrNoXBRLFacts = errors.New("edgar: the filer has published no XBRL facts")

// CompanyFacts is one filer's whole tagged history, taxonomy by tag.
type CompanyFacts struct {
	CIK        int                           `json:"cik"`
	EntityName string                        `json:"entityName"`
	Facts      map[string]map[string]Concept `json:"facts"`
}

// Concept is one tag's values, keyed by the unit they are measured in.
type Concept struct {
	Label string              `json:"label"`
	Units map[string]FactList `json:"units"`
}

// FactList is a unit's values; it decodes an empty OBJECT as well as an array, since
// routes answer `"units":{"USD":{}}` for a tag declared and never reported.
type FactList []Fact

func (l *FactList) UnmarshalJSON(b []byte) error {
	if s := bytes.TrimSpace(b); len(s) > 0 && s[0] == '{' {
		*l = nil
		return nil
	}
	var out []Fact
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	*l = out
	return nil
}

// Fact is one reported value. Start is empty for an instant - a balance-sheet
// line - and set for a duration, which is what an income line is.
type Fact struct {
	Start string  `json:"start"`
	End   string  `json:"end"`
	Val   float64 `json:"val"`
	Accn  string  `json:"accn"`
	FY    int     `json:"fy"`
	FP    string  `json:"fp"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
	// Tag is the concept this fact was read out of, carried on the fact because
	// a caller resolving a candidate list has to say which name answered.
	Tag string `json:"-"`
}

// Taxonomy is the standard one. A filer's own extension namespace, which is
// where a stated FFO total would be, is not published here at all.
const Taxonomy = "us-gaap"

// CompanyFacts reads one filer's whole tagged history. The CIK is padded to ten
// digits here, unlike the archive path, which takes it unpadded.
func (c *Client) CompanyFacts(ctx context.Context, cik int) (CompanyFacts, error) {
	url := fmt.Sprintf("%s/api/xbrl/companyfacts/CIK%010d.json", DataBase, cik)
	b, status, err := c.fetchStatus(ctx, url)
	if status == http.StatusNotFound {
		return CompanyFacts{}, fmt.Errorf("%w: CIK %d", ErrNoXBRLFacts, cik)
	}
	if err != nil {
		return CompanyFacts{}, err
	}
	var out CompanyFacts
	if err := decodeJSON(b, url, &out); err != nil {
		return CompanyFacts{}, err
	}
	return out, nil
}

// The bounds on a reported duration that make it a FISCAL YEAR: a 52/53-week
// filer's year is not 365 days, and a three-quarter cumulative figure is not one.
const (
	minAnnualDays = 350
	maxAnnualDays = 380
)

// AnnualUSD is one tag's annual figures keyed by ISO period end; a year restated by
// a later filing wins, as EDGAR carries every filing's version.
func (f CompanyFacts) AnnualUSD(taxonomy, tag string) map[string]Fact {
	out := map[string]Fact{}
	con, ok := f.Facts[taxonomy][tag]
	if !ok {
		return out
	}
	for _, fact := range con.Units["USD"] {
		if !fact.annual() {
			continue
		}
		if prev, seen := out[fact.End]; seen && prev.Filed >= fact.Filed {
			continue
		}
		fact.Tag = tag
		out[fact.End] = fact
	}
	return out
}

// annual reports whether this fact covers about a year.
func (f Fact) annual() bool {
	start, ok := docfmt.ParseISODay(f.Start)
	if !ok {
		return false
	}
	end, ok := docfmt.ParseISODay(f.End)
	if !ok {
		return false
	}
	days := end.Sub(start).Hours() / 24
	return days >= minAnnualDays && days <= maxAnnualDays
}
