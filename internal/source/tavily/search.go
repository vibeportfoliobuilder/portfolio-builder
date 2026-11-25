package tavily

// search.go - the one route. A query is a question, so the answer is ranked
// rather than keyed, and nothing here is addressed by a broker id or ticker.

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// searchPath is the only route this package calls.
const searchPath = "/search"

// TopicNews asks for news rather than general web results, which is the only
// way the origin returns a publication date. Measured 2026-09-24.
const TopicNews = "news"

// DefaultMaxResults is what a query asks for when it does not say.
const DefaultMaxResults = 5

// maxResultsCeiling bounds what a caller may ask for, so a model cannot turn one
// tool call into a hundred rows of ledger.
const maxResultsCeiling = 20

// ErrEmptyQuery is a query with nothing in it, refused here rather than spent.
var ErrEmptyQuery = errors.New("tavily: empty query")

// Query is one search. Topic is empty for general web results, or TopicNews.
type Query struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
	Topic      string `json:"topic,omitempty"`
}

// Hit is one ranked result. PublishedDate is populated only under TopicNews,
// where the origin sends RFC1123 ("Wed, 23 Sep 2026 17:21:12 GMT").
type Hit struct {
	URL           string
	Title         string
	Content       string
	Score         float64
	PublishedDate string
}

// Result is one query's answer, carrying the query the origin echoed back.
type Result struct {
	Query string
	Hits  []Hit
}

// Search runs one query. The caller's max_results is clamped rather than
// refused: a model asking for eighty is asking clumsily, not wrongly.
func (c *Client) Search(ctx context.Context, q Query) (Result, error) {
	q.Query = strings.TrimSpace(q.Query)
	if q.Query == "" {
		return Result{}, ErrEmptyQuery
	}
	if q.MaxResults <= 0 {
		q.MaxResults = DefaultMaxResults
	}
	if q.MaxResults > maxResultsCeiling {
		q.MaxResults = maxResultsCeiling
	}
	body, err := marshal(q)
	if err != nil {
		return Result{}, err
	}
	raw, err := c.post(ctx, searchPath, cacheKey(body), body)
	if err != nil {
		return Result{}, fmt.Errorf("tavily: %q: %w", q.Query, err)
	}
	return decodeSearch(raw)
}
