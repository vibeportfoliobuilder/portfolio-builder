package livetool

// search.go - web_search. It searches, writes the ledger, and only then
// answers: a result the model can cite but the audit cannot see is the failure.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/tavily"
)

// ToolWebSearch is the tool's registered name, on both providers.
const ToolWebSearch = "web_search"

// snippetLimit bounds what one result contributes to sources.yaml. The ledger
// is a citation record and not a copy of the page.
const snippetLimit = 400

// SearchArgs is web_search's input schema. The jsonschema tag is
// comma-separated, so every description here must stay comma-free.
type SearchArgs struct {
	Query      string `json:"query" jsonschema:"required,description=What to search for. One question in plain words - not a boolean expression and not a URL."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"description=How many results to return. Defaults to 5 and is capped; ask for more only when the first pass was genuinely too narrow."`
}

// Searcher runs web_search against one workspace. dir is where sources.yaml
// lives, and now is the instant already threaded through the run.
type Searcher struct {
	client *tavily.Client
	dir    string
	now    time.Time
}

// NewSearcher binds a search client to the directory its citations land in.
func NewSearcher(client *tavily.Client, dir string, now time.Time) *Searcher {
	return &Searcher{client: client, dir: dir, now: now}
}

// Description is what the model is told the tool does, including the fact that
// every result is recorded - which is the behaviour it has to cite against.
func (s *Searcher) Description() string {
	return "Search the web for judgement you cannot get from your inputs - industry structure, " +
		"a competitor's position, a CEO's prior record. Every result is written to " + LedgerFile +
		" in this run's workspace before it reaches you, so cite one by its URL and a reader can " +
		"check it. Prefer your declared inputs: this reaches outside the snapshot and costs a call."
}

// Search answers one query, having first recorded every result it will return.
func (s *Searcher) Search(ctx context.Context, a SearchArgs) (string, error) {
	res, err := s.client.Search(ctx, tavily.Query{Query: a.Query, MaxResults: a.MaxResults})
	if err != nil {
		return "", err
	}
	if len(res.Hits) == 0 {
		return fmt.Sprintf("No results for %q. Record the gap rather than inferring around it.", a.Query), nil
	}

	rows := make([]Source, 0, len(res.Hits))
	for _, h := range res.Hits {
		rows = append(rows, Source{
			URL:     h.URL,
			Title:   h.Title,
			Snippet: clip(h.Content, snippetLimit),
			Queries: []string{a.Query},
		})
	}
	// Written BEFORE the results are returned. CONTRIBUTE, "A tool that reaches
	// outside writes down what it found".
	if _, err := Append(s.dir, s.now, rows); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d results for %q, all recorded in %s:\n\n", len(res.Hits), a.Query, LedgerFile)
	for i, h := range res.Hits {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, h.Title, h.URL)
		if h.PublishedDate != "" {
			fmt.Fprintf(&b, "   published: %s\n", h.PublishedDate)
		}
		fmt.Fprintf(&b, "   %s\n\n", clip(h.Content, snippetLimit))
	}
	b.WriteString("Cite any figure you take from these by its URL, and say so in your Sources section.")
	return b.String(), nil
}

// clipNoted is clip at limit, followed by a line saying how much of the text was
// shown wherever the limit cut it: a body handed to the model whole or marked.
func clipNoted(text string, limit int) string {
	out := clip(text, limit)
	if n := len([]rune(text)); n > limit {
		out += fmt.Sprintf("\n\n[truncated: %d of %d characters shown]", limit, n)
	}
	return out
}

// clip shortens s to n runes on a rune boundary, so a multi-byte character is
// never cut in half.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "..."
}
