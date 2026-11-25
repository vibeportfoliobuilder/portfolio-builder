package edgar

// ffo_test.go - FFO assembled from a made-up filer's tagged lines: per period, the first tag that
// answers, a missing optional line noted, and a missing required line refusing the sum.

import (
	"context"
	"os"
	"strings"
	"testing"
)

func annual(start, end string, val float64, filed string) Fact {
	return Fact{Start: start, End: end, Val: val, Form: "10-K", FP: "FY", Filed: filed}
}

func facts(tags map[string][]Fact) CompanyFacts {
	c := CompanyFacts{CIK: 1, EntityName: "Example Realty", Facts: map[string]map[string]Concept{Taxonomy: {}}}
	for tag, fs := range tags {
		c.Facts[Taxonomy][tag] = Concept{Units: map[string]FactList{"USD": fs}}
	}
	return c
}

func TestFFOIsAssembledPerPeriod(t *testing.T) {
	c := facts(map[string][]Fact{
		// The filer switched its net income tag: 2024 under NetIncomeLoss, 2025 only under ProfitLoss.
		"NetIncomeLoss": {annual("2024-01-01", "2024-12-31", 100, "2025-02-01")},
		"ProfitLoss":    {annual("2025-01-01", "2025-12-31", 120, "2026-02-01")},
		"DepreciationDepletionAndAmortization": {
			annual("2024-01-01", "2024-12-31", 50, "2025-02-01"),
			annual("2025-01-01", "2025-12-31", 55, "2026-02-01"),
			{Start: "2025-10-01", End: "2025-12-31", Val: 14, Form: "10-Q", Filed: "2026-02-01"}, // a quarter: not annual
		},
		"GainLossOnSaleOfProperties": {annual("2025-01-01", "2025-12-31", 10, "2026-02-01")},
	})
	years, notes := FundsFromOperations(c)
	if len(years) != 2 || years["2024-12-31"].FFO != 150 || years["2025-12-31"].FFO != 165 {
		t.Fatalf("years %+v", years)
	}
	if got := strings.Join(notes, " | "); !strings.Contains(got, "impairments line") || !strings.Contains(got, "ProfitLoss") {
		t.Errorf("notes %q", got)
	}
}

func TestFFOWithoutDepreciationIsRefused(t *testing.T) {
	years, notes := FundsFromOperations(facts(map[string][]Fact{"NetIncomeLoss": {annual("2025-01-01", "2025-12-31", 100, "2026-02-01")}}))
	if years != nil || len(notes) != 1 || !strings.Contains(notes[0], "depreciation") {
		t.Errorf("%v %v", years, notes)
	}
}

// TestLiveRegistrant reads one public filer (make test-live sets PB_LIVE_EDGAR; set SEC_USER_AGENT
// to your own name and email, as the SEC asks).
func TestLiveRegistrant(t *testing.T) {
	if os.Getenv("PB_LIVE_EDGAR") == "" || os.Getenv(UserAgentEnv) == "" {
		t.Skip("set PB_LIVE_EDGAR=1 and SEC_USER_AGENT to read EDGAR")
	}
	c := New(nil)
	cik, err := c.CIK(context.Background(), "KO")
	if err != nil || cik == 0 {
		t.Fatalf("cik %d %v", cik, err)
	}
	f, err := c.CompanyFacts(context.Background(), cik)
	if err != nil || len(f.AnnualUSD(Taxonomy, "NetIncomeLoss")) == 0 {
		t.Errorf("facts: %v", err)
	}
}
