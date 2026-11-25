package store

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
)

// ManifestSummary counts cache entries by status, for a run's report.
func ManifestSummary(m core.Manifest) string {
	counts := map[string]int{}
	for _, e := range m.Entries {
		counts[e.Status]++
	}
	keys := slices.Sorted(maps.Keys(counts))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}
