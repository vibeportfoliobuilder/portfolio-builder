package edgar

// ffo.go - FFO assembled from four tagged lines. THE TAG A FILER USES IS NOT STABLE
// (American Tower dropped NetIncomeLoss after 2020): lines resolve per PERIOD against candidates.

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
)

// ffoLine is one addend of the definition: the candidates in the order they are
// tried, and whether the sum can be struck without it.
type ffoLine struct {
	name     string
	sign     float64
	required bool
	tags     []string
}

// ffoLines is the definition: net income, plus real-estate depreciation, plus
// impairments, less gains on property sales.
var ffoLines = []ffoLine{{
	name: "net income", sign: 1, required: true,
	tags: []string{"NetIncomeLoss", "ProfitLoss", "NetIncomeLossAvailableToCommonStockholdersBasic"},
}, {
	name: "depreciation and amortisation", sign: 1, required: true,
	tags: []string{
		"DepreciationDepletionAndAmortization",
		"DepreciationAmortizationAndAccretionNet",
		"DepreciationAndAmortization",
		"RealEstateInvestmentPropertyDepreciation",
		// One net-lease REIT tags only Depreciation (its leases are mostly sales-type, so it is small).
		"Depreciation",
	},
}, {
	name: "impairments", sign: 1,
	tags: []string{
		"ImpairmentOfRealEstate",
		"AssetImpairmentCharges",
		"ImpairmentOfLongLivedAssetsHeldForUse",
		"ImpairmentOfLongLivedAssetsToBeDisposedOf",
	},
}, {
	name: "gains on property sales", sign: -1,
	tags: []string{
		"GainLossOnSaleOfProperties",
		"GainsLossesOnSalesOfInvestmentRealEstate",
		"GainLossOnSaleOfPropertiesNetOfApplicableIncomeTaxes",
		"GainLossOnDispositionOfAssets1",
		"GainLossOnDispositionOfProperty",
		"GainLossOnSaleOfPropertyPlantEquipment",
		"GainLossOnSaleOfRealEstateInvestmentProperty",
	},
}}

// NoteFFOAssembled names the tags that answered, for a figure that IS filled:
// provenance rather than work anybody can do, which is what makes it a note.
const NoteFFOAssembled = "edgar: funds from operations for %s were assembled from the filer's own tagged figures: %s"

// FFOYear is one fiscal year's funds from operations, in the filer's own units,
// with the tag each line was resolved to. A line nobody tagged is absent from Lines.
type FFOYear struct {
	PeriodEnd string
	FFO       float64
	Lines     []FFOLineValue
}

// FFOLineValue is one addend as it was found: the tag that answered and the
// signed contribution it made, so the sum can be checked by adding them up.
type FFOLineValue struct {
	Name string
	Tag  string
	Val  float64
}

// FundsFromOperations reads one filer's annual FFO by period end; the notes name the
// lines nobody tagged, since a definition without its impairment line is another figure.
func FundsFromOperations(facts CompanyFacts) (map[string]FFOYear, []string) {
	resolved := make([]map[string]Fact, len(ffoLines))
	var missing []string
	for i, line := range ffoLines {
		resolved[i] = map[string]Fact{}
		for _, tag := range line.tags {
			for end, f := range facts.AnnualUSD(Taxonomy, tag) {
				if _, taken := resolved[i][end]; !taken {
					resolved[i][end] = f
				}
			}
		}
		if len(resolved[i]) == 0 {
			missing = append(missing, line.name)
		}
	}
	if len(resolved[0]) == 0 || len(resolved[1]) == 0 {
		return nil, []string{fmt.Sprintf(
			"edgar: funds from operations could not be assembled from the filer's own tagged figures - %s carries no annual value under any of the names this program knows, so the definition cannot be struck at all",
			strings.Join(missing, " and "))}
	}

	out := map[string]FFOYear{}
	for end := range resolved[0] {
		if _, ok := resolved[1][end]; !ok {
			continue
		}
		y := FFOYear{PeriodEnd: end}
		for i, line := range ffoLines {
			f, ok := resolved[i][end]
			if !ok {
				continue
			}
			v := line.sign * f.Val
			y.FFO += v
			y.Lines = append(y.Lines, FFOLineValue{Name: line.name, Tag: f.Tag, Val: v})
		}
		sort.Slice(y.Lines, func(a, b int) bool { return y.Lines[a].Name < y.Lines[b].Name })
		out[end] = y
	}
	notes := []string{fmt.Sprintf(NoteFFOAssembled, core.Plural(len(out), "fiscal year"), describeFFO(out))}
	if len(missing) > 0 {
		notes = append(notes, fmt.Sprintf(
			"edgar: the %s line of the funds-from-operations definition is not tagged by this filer under any name this program knows, so the sum is struck without it",
			strings.Join(missing, " and the ")))
	}
	return out, notes
}

// describeFFO names the tags that answered, once each, so the sentence says
// which spelling of a line this filer uses rather than only that it has one.
func describeFFO(years map[string]FFOYear) string {
	seen := map[string]bool{}
	for _, y := range years {
		for _, l := range y.Lines {
			seen[l.Tag] = true
		}
	}
	return strings.Join(slices.Sorted(maps.Keys(seen)), ", ")
}
