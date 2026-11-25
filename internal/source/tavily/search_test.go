package tavily

// search_test.go - the search route against a made-up server: the request carries the key and a
// clamped max_results, the answer decodes into hits, and an empty answer is not an error.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fakeServer(t *testing.T, answer string, seen *map[string]any) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != searchPath || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, seen)
		(*seen)["auth"] = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), Base: srv.URL, key: "made-up-key", Backoff: []time.Duration{}}
}

func TestSearchDecodesHits(t *testing.T) {
	seen := map[string]any{}
	c := fakeServer(t, `{"query":"example dividend","results":[`+
		`{"url":"https://example.com/a","title":"A","content":"first","score":0.9,"published_date":"Wed, 23 Sep 2026 17:21:12 GMT"},`+
		`{"url":"","title":"no url"},`+
		`{"url":"https://example.com/b","title":"B","content":"second","score":0.5}],"answer":"ignored"}`, &seen)
	r, err := c.Search(context.Background(), Query{Query: "  example dividend ", MaxResults: 80})
	if err != nil {
		t.Fatal(err)
	}
	if r.Query != "example dividend" || len(r.Hits) != 2 || r.Hits[0].URL != "https://example.com/a" || r.Hits[0].PublishedDate == "" {
		t.Errorf("%+v (a hit with no URL is dropped)", r)
	}
	if seen["max_results"] != float64(maxResultsCeiling) || !strings.Contains(seen["auth"].(string), "made-up-key") {
		t.Errorf("request %v", seen)
	}
}

func TestNothingFoundIsAnAnswer(t *testing.T) {
	seen := map[string]any{}
	r, err := fakeServer(t, `{"query":"q","results":[]}`, &seen).Search(context.Background(), Query{Query: "q"})
	if err != nil || len(r.Hits) != 0 {
		t.Errorf("%+v %v", r, err)
	}
	if _, err := (&Client{}).Search(context.Background(), Query{Query: "  "}); err != ErrEmptyQuery {
		t.Errorf("empty query: %v", err)
	}
}

// TestLiveSearch spends one query (make test-live sets PB_LIVE_TAVILY; needs TAVILY_API_KEY).
func TestLiveSearch(t *testing.T) {
	if os.Getenv("PB_LIVE_TAVILY") == "" {
		t.Skip("set PB_LIVE_TAVILY=1 and TAVILY_API_KEY to spend one query")
	}
	c, err := NewClient()
	if err != nil {
		t.Skip(err)
	}
	r, err := c.Search(context.Background(), Query{Query: "Coca-Cola dividend increase", MaxResults: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hits) == 0 || r.Hits[0].URL == "" {
		t.Errorf("%+v", r)
	}
}
