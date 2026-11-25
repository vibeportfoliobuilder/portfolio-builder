package livetool

// ledger.go - sources.yaml, the citation ledger a live tool appends to BEFORE
// its result reaches the model. Append-only, and a re-run is a zero diff.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/docfmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// LedgerFile is the name every live tool appends its citations to, beside the
// dossier it is reasoning about.
const LedgerFile = "sources.yaml"

// dateLayout is the shape docfmt.FormatDate writes retrieved_at in: a date, not
// an instant, since a time of day would diff on every re-run.
const dateLayout = "2006-01-02"

// Source is one citation: where bytes came from, and the day they were FIRST
// seen. First-seen, because a later sighting must not rewrite a stable file.
type Source struct {
	URL         string   `yaml:"url"`
	Title       string   `yaml:"title,omitempty"`
	Publisher   string   `yaml:"publisher,omitempty"`
	RetrievedAt string   `yaml:"retrieved_at,omitempty"`
	Snippet     string   `yaml:"snippet,omitempty"`
	Queries     []string `yaml:"queries,omitempty"`
	// TextFile is the readable text fetch_url returned, saved beside the ledger (sources/<hash>.txt)
	// so a claim cited from a long document can be audited.
	TextFile string `yaml:"text_file,omitempty"`
}

// Ledger is the on-disk shape of sources.yaml.
type Ledger struct {
	Sources []Source `yaml:"sources,omitempty"`
}

// ErrLedgerShrank reports a merge that would drop a URL. The file is an
// archive, so losing a row is a bug rather than an update.
var ErrLedgerShrank = errors.New("livetool: sources.yaml would lose a url")

// ReadLedger reads dir's sources.yaml. A missing file is an empty ledger: the
// first tool call of a run has nothing to read and that is not an error.
func ReadLedger(dir string) (Ledger, error) { return readYAML[Ledger](dir, LedgerFile) }

// Append merges rows into dir's sources.yaml and reports how many URLs were new.
// now dates the new rows, and must be the instant already threaded through the call.
func Append(dir string, now time.Time, rows []Source) (int, error) {
	old, err := ReadLedger(dir)
	if err != nil {
		return 0, err
	}
	merged, added, err := merge(old, rows, now)
	if err != nil {
		return 0, err
	}
	if err := writeYAML(dir, LedgerFile, merged); err != nil {
		return 0, err
	}
	return added, nil
}

// readYAML decodes dir's file name into a T. An absent file is the zero T, and
// a file that does not read or decode is an error naming it.
func readYAML[T any](dir, name string) (T, error) {
	var v T
	b, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return v, fmt.Errorf("livetool: reading %s: %w", name, err)
	}
	if err := yaml.Unmarshal(b, &v); err != nil {
		var zero T
		return zero, fmt.Errorf("livetool: decoding %s: %w", name, err)
	}
	return v, nil
}

// writeYAML writes v as dir's file name through the one YAML encoder and the one
// atomic write, either failure an error naming the file.
func writeYAML(dir, name string, v any) error {
	b, err := docfmt.MarshalYAML(v)
	if err != nil {
		return fmt.Errorf("livetool: encoding %s: %w", name, err)
	}
	if err := store.WriteFileAtomic(filepath.Join(dir, name), b); err != nil {
		return fmt.Errorf("livetool: writing %s: %w", name, err)
	}
	return nil
}

// merge folds rows into old by URL and returns the ledger sorted by URL, so the
// same set of citations always encodes to the same bytes.
func merge(old Ledger, rows []Source, now time.Time) (Ledger, int, error) {
	by := make(map[string]Source, len(old.Sources)+len(rows))
	for _, s := range old.Sources {
		by[s.URL] = s
	}
	before := len(by)
	stamp := docfmt.FormatDate(now)
	for _, r := range rows {
		r.URL = strings.TrimSpace(r.URL)
		if r.URL == "" {
			continue
		}
		cur, seen := by[r.URL]
		if !seen {
			if r.RetrievedAt == "" {
				r.RetrievedAt = stamp
			}
			r.Queries = tidy(r.Queries)
			by[r.URL] = r
			continue
		}
		by[r.URL] = fold(cur, r)
	}
	out := Ledger{Sources: make([]Source, 0, len(by))}
	for _, s := range by {
		out.Sources = append(out.Sources, s)
	}
	slices.SortFunc(out.Sources, func(a, b Source) int { return strings.Compare(a.URL, b.URL) })
	// By URL, not by row count: a hand-edited file that lists one URL twice is
	// deduped here, and that is not the loss this guard is looking for.
	for _, was := range old.Sources {
		if _, still := by[was.URL]; !still {
			return Ledger{}, 0, fmt.Errorf("%w: %s", ErrLedgerShrank, was.URL)
		}
	}
	return out, len(by) - before, nil
}

// fold keeps what the ledger already recorded and fills only what was blank:
// a second sighting of a URL must not restate its retrieval date.
func fold(cur, next Source) Source {
	if cur.Title == "" {
		cur.Title = next.Title
	}
	if cur.Publisher == "" {
		cur.Publisher = next.Publisher
	}
	if cur.Snippet == "" {
		cur.Snippet = next.Snippet
	}
	if cur.RetrievedAt == "" {
		cur.RetrievedAt = next.RetrievedAt
	}
	if next.TextFile != "" {
		cur.TextFile = next.TextFile
	}
	cur.Queries = tidy(append(cur.Queries, next.Queries...))
	return cur
}

// tidy sorts and dedupes a query list, dropping blanks. Order out of a map or a
// vendor response is not stable, and the file has to be.
func tidy(in []string) []string {
	out := make([]string, 0, len(in))
	for _, q := range in {
		if q = strings.TrimSpace(q); q != "" {
			out = append(out, q)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
