package tavily

// decode.go - the response shape, measured live 2026-09-24. raw_content is
// decoded and dropped: this package never carries a page body into the caller.

import (
	"encoding/json"
	"fmt"
)

// searchResponse is the envelope. answer, images, follow_up_questions and
// request_id are present and deliberately unread.
type searchResponse struct {
	Query   string `json:"query"`
	Results []struct {
		URL           string  `json:"url"`
		Title         string  `json:"title"`
		Content       string  `json:"content"`
		Score         float64 `json:"score"`
		PublishedDate string  `json:"published_date"`
	} `json:"results"`
}

// decodeSearch reads the envelope. A 200 with no results is a Result with no
// hits, not an error: "nothing found" is an answer the caller has to record.
func decodeSearch(raw []byte) (Result, error) {
	var r searchResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf("tavily: decoding the response: %w", err)
	}
	out := Result{Query: r.Query, Hits: make([]Hit, 0, len(r.Results))}
	for _, h := range r.Results {
		if h.URL == "" {
			continue
		}
		out.Hits = append(out.Hits, Hit{
			URL:           h.URL,
			Title:         h.Title,
			Content:       h.Content,
			Score:         h.Score,
			PublishedDate: h.PublishedDate,
		})
	}
	return out, nil
}
