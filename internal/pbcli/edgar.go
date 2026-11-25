package pbcli

// edgar.go - pb edgar (SEC filings and REIT FFO for US filers), and the human_view.yaml schema
// that a research connector (or the user, or the cited web fallback) fills.

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/edgar"
)

// HumanView is data/stocks/<T>/human_view.yaml: a human analyst's moat and fair value (house
// rule 2), from a research connector, the user's own file or a cited web source. No agent may
// replace these with its own moat rating or fair value. SourceURL and Summary are set when the
// view was read from the web: the link and a short summary in the agent's own words. Rating is
// the house's own overall label, as it states it ("4 stars", "Buy", "Outperform").
type HumanView struct {
	AsOf   string `yaml:"as_of" json:"as_of"`
	Source string `yaml:"source" json:"source"`
	// BrokerID is the name's instrument id at the broker, for its news feed (a connector's).
	BrokerID string `yaml:"broker_id,omitempty" json:"broker_id,omitempty"`
	// Covered says a human analyst states the moat or fair value. Absent, a file with a moat or a
	// fair value counts as covered; false (a model-only report) never does.
	Covered           *bool     `yaml:"covered,omitempty" json:"covered,omitempty"`
	ReportDate        string    `yaml:"report_date,omitempty" json:"report_date,omitempty"`
	Rating            string    `yaml:"rating,omitempty" json:"rating,omitempty"`
	Moat              string    `yaml:"moat,omitempty" json:"moat,omitempty"`
	MoatTrend         string    `yaml:"moat_trend,omitempty" json:"moat_trend,omitempty"`
	FairValue         float64   `yaml:"fair_value,omitempty" json:"fair_value,omitempty"`
	FairValueCurrency string    `yaml:"fair_value_currency,omitempty" json:"fair_value_currency,omitempty"`
	FairValueAsOf     string    `yaml:"fair_value_as_of,omitempty" json:"fair_value_as_of,omitempty"`
	Uncertainty       string    `yaml:"uncertainty,omitempty" json:"uncertainty,omitempty"`
	CapitalAllocation string    `yaml:"capital_allocation,omitempty" json:"capital_allocation,omitempty"`
	BullCase          []string  `yaml:"bull_case,omitempty" json:"bull_case,omitempty"`
	BearCase          []string  `yaml:"bear_case,omitempty" json:"bear_case,omitempty"`
	CasesAsOf         string    `yaml:"cases_as_of,omitempty" json:"cases_as_of,omitempty"`
	Commentary        []Comment `yaml:"commentary,omitempty" json:"commentary,omitempty"`
	// Coverage is "analyst" or "quantitative". A quantitative report has no analyst moat or fair
	// value: Covered is false, and the model's figures sit only in Quantitative, never in moat/fair_value.
	Coverage      string     `yaml:"coverage,omitempty" json:"coverage,omitempty"`
	Quantitative  *QuantView `yaml:"quantitative,omitempty" json:"quantitative,omitempty"`
	FairValueNote string     `yaml:"fair_value_note,omitempty" json:"fair_value_note,omitempty"`
	MoatTrendNote string     `yaml:"moat_trend_note,omitempty" json:"moat_trend_note,omitempty"`
	SourceURL     string     `yaml:"source_url,omitempty" json:"source_url,omitempty"`
	Summary       string     `yaml:"summary,omitempty" json:"summary,omitempty"`
}

// IsCovered reports a view a human analyst stands behind (see Covered).
func (h HumanView) IsCovered() bool {
	if h.Covered != nil {
		return *h.Covered
	}
	return h.Moat != "" || h.FairValue > 0
}

// QuantView is a research house's quantitative (model) rating, labelled so: not a human view.
type QuantView struct {
	Rating            string  `yaml:"rating,omitempty" json:"rating,omitempty"`
	Moat              string  `yaml:"moat,omitempty" json:"moat,omitempty"`
	FairValue         float64 `yaml:"fair_value,omitempty" json:"fair_value,omitempty"`
	FairValueCurrency string  `yaml:"fair_value_currency,omitempty" json:"fair_value_currency,omitempty"`
	FairValueAsOf     string  `yaml:"fair_value_as_of,omitempty" json:"fair_value_as_of,omitempty"`
	Uncertainty       string  `yaml:"uncertainty,omitempty" json:"uncertainty,omitempty"`
	FinancialHealth   string  `yaml:"financial_health,omitempty" json:"financial_health,omitempty"`
	Note              string  `yaml:"note" json:"note"`
}

// finite turns NaN or infinity (an absent figure in the decoder) into 0, which omitempty leaves out.
func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// Comment is one titled block of the analyst note, clipped.
type Comment struct {
	Title string `yaml:"title" json:"title"`
	Date  string `yaml:"date,omitempty" json:"date,omitempty"`
	Text  string `yaml:"text" json:"text"`
}

// commentLimit keeps human_view.yaml small: the agents read the headline fields first.
const commentLimit = 1500

func day(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func clipText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + " …"
}

// EdgarDoc is data/stocks/<T>/edgar.yaml.
type EdgarDoc struct {
	AsOf    string        `yaml:"as_of" json:"as_of"`
	Source  string        `yaml:"source" json:"source"`
	CIK     int           `yaml:"cik" json:"cik"`
	Name    string        `yaml:"name" json:"name"`
	SIC     string        `yaml:"sic,omitempty" json:"sic,omitempty"`
	FYEnd   string        `yaml:"fiscal_year_end,omitempty" json:"fiscal_year_end,omitempty"`
	Filings []EdgarFiling `yaml:"filings" json:"filings"`
	FFO     []EdgarFFO    `yaml:"ffo,omitempty" json:"ffo,omitempty"`
	Notes   []string      `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// EdgarFiling is one filing with its document and index addresses.
type EdgarFiling struct {
	Form   string `yaml:"form" json:"form"`
	Filed  string `yaml:"filed" json:"filed"`
	Period string `yaml:"period,omitempty" json:"period,omitempty"`
	Items  string `yaml:"items,omitempty" json:"items,omitempty"`
	URL    string `yaml:"url" json:"url"`
	Index  string `yaml:"index_url,omitempty" json:"index_url,omitempty"`
}

// EdgarFFO is one fiscal year's funds from operations, assembled from XBRL tags, in millions of USD.
type EdgarFFO struct {
	PeriodEnd string  `yaml:"period_end" json:"period_end"`
	FFOm      float64 `yaml:"ffo_millions" json:"ffo_millions"`
}

func edgarCmd(e *env) *cobra.Command {
	var ffo bool
	c := &cobra.Command{
		Use:   "edgar <TICKER>",
		Short: "SEC filings (latest 10-K, 10-Qs, proxy, 8-Ks) and REIT FFO into data/stocks/<T>/edgar.yaml",
		Long: `For US filers only. Lists the latest 10-K, the last three 10-Qs, the latest DEF 14A and the last
five 8-Ks with their document URLs (open one with pb fetch-url). --ffo (default on for REITs in
financial_data.yaml) assembles annual FFO from the company's XBRL facts; pb derive then judges a
REIT's payout on FFO. No credential; SEC_USER_AGENT overrides the default User-Agent.
Example:
  pb edgar KO`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ts, err := e.targets(args, false, kindStock)
			if err != nil {
				return err
			}
			t := ts[0]
			cl := edgar.New(nil)
			reg, err := cl.Registrant(cmd.Context(), symbolOf(t))
			if err != nil {
				if errors.Is(err, edgar.ErrNotARegistrant) {
					return fmt.Errorf("%w: EDGAR covers US filers only", err)
				}
				return err
			}
			sub, err := cl.Submissions(cmd.Context(), reg.CIK)
			if err != nil {
				return err
			}
			doc := EdgarDoc{AsOf: e.today(), Source: "sec edgar", CIK: reg.CIK, Name: sub.Name, SIC: sub.SICDesc, FYEnd: sub.FYEnd}
			add := func(f edgar.Filing) {
				doc.Filings = append(doc.Filings, EdgarFiling{Form: f.Form, Filed: day(f.Filed), Period: day(f.Period), Items: f.Items, URL: f.DocURL, Index: f.IndexURL})
			}
			for _, form := range []string{"10-K", "DEF 14A"} {
				if f, ok := sub.Latest(form); ok {
					add(f)
				}
			}
			for _, f := range sub.Recent("10-Q", 3) {
				add(f)
			}
			for _, f := range sub.Recent("8-K", 5) {
				add(f)
			}
			// A foreign private issuer (an ADR) files a 20-F (40-F from Canada) and 6-Ks instead.
			if len(doc.Filings) == 0 {
				for _, form := range []string{"20-F", "40-F"} {
					if f, ok := sub.Latest(form); ok {
						add(f)
					}
				}
				for _, f := range sub.Recent("6-K", 5) {
					add(f)
				}
				if len(doc.Filings) > 0 {
					doc.Notes = append(doc.Notes, "foreign private issuer: the annual report is the 20-F (or 40-F), interim news the 6-K")
				}
			}
			if !cmd.Flags().Changed("ffo") {
				ffo = e.isREIT(t.Ticker)
			}
			if ffo {
				facts, err := cl.CompanyFacts(cmd.Context(), reg.CIK)
				if err != nil {
					doc.Notes = append(doc.Notes, "company facts unreadable: "+err.Error())
				} else {
					years, notes := edgar.FundsFromOperations(facts)
					doc.Notes = append(doc.Notes, notes...)
					for _, y := range years {
						doc.FFO = append(doc.FFO, EdgarFFO{PeriodEnd: y.PeriodEnd, FFOm: round2v(y.FFO / 1e6)})
					}
					slices.SortFunc(doc.FFO, func(a, b EdgarFFO) int { return strings.Compare(b.PeriodEnd, a.PeriodEnd) })
				}
			}
			if _, err := e.writeYAML(doc, "stocks", t.Ticker, "edgar.yaml"); err != nil {
				return err
			}
			return e.printJSON(doc)
		},
	}
	c.Flags().BoolVar(&ffo, "ffo", false, "assemble annual FFO from XBRL (default: on when the name is a REIT)")
	return c
}
