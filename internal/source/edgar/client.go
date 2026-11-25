package edgar

// client.go - the two documents this package reads, and the one header that
// makes either of them answer.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/httpx"
)

// The two origins: www.sec.gov serves the ticker map and the documents,
// data.sec.gov the submissions index. Vars, so a test can redirect them.
const (
	WWWHost  = "www.sec.gov"
	DataHost = "data.sec.gov"
)

var (
	WWWBase  = "https://" + WWWHost
	DataBase = "https://" + DataHost
)

// DefaultPace is the SEC's published ten a second plus 15%. It held 2,608
// requests over five minutes without a refusal, measured 2026-10-04.
const DefaultPace = 115 * time.Millisecond

// UserAgentEnv lets an operator put their own name and address in the header
// the SEC's fair-use policy asks for.
const UserAgentEnv = "SEC_USER_AGENT"

// defaultUserAgent is what the header says when the environment says nothing.
// Probed 2026-08-25: it must carry an @-shaped token, and one naming github.com is 403.
const defaultUserAgent = "portfolio-builder/1.0 (unset-contact@portfolio-builder.invalid)"

// ErrNotARegistrant means the SEC's ticker map does not carry this symbol - a
// sentinel, because a European line files with its own regulator.
var ErrNotARegistrant = errors.New("edgar: not in the SEC's ticker map")

// Client is one run's access to EDGAR. The ticker map is fetched at most once
// and held for the life of the client: it is one 800 KB document for the market.
type Client struct {
	HC        *http.Client
	Pacer     *httpx.Pacer
	UserAgent string

	// Backoff is this client's retry schedule: the wait before each retry, and by
	// its length how many there are. nil is httpx.DefaultBackoff().
	Backoff []time.Duration

	tickersOnce sync.Once
	tickers     map[string]Registrant
	tickersErr  error
}

// New returns a client with this package's pacing and header.
func New(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{HC: hc, Pacer: httpx.NewPacer(DefaultPace), UserAgent: userAgent()}
}

func userAgent() string {
	if s := strings.TrimSpace(os.Getenv(UserAgentEnv)); s != "" {
		return s
	}
	return defaultUserAgent
}

// get reads one JSON document, through the pacer and with the required header.
// No conditional GET or cache: this package asks for each document once a run.
func (c *Client) get(ctx context.Context, url string, into any) error {
	b, err := c.fetch(ctx, url)
	if err != nil {
		return err
	}
	return decodeJSON(b, url, into)
}

// decodeJSON names the document in the error, which is the whole difference
// between a decode failure a reader can chase and one they cannot.
func decodeJSON(b []byte, url string, into any) error {
	if err := json.Unmarshal(b, into); err != nil {
		return fmt.Errorf("edgar: %s: %w", url, err)
	}
	return nil
}

// Registrant is who the SEC's ticker map says holds a symbol, which is a US
// listing's holder and may be nobody a European line with that symbol knows.
type Registrant struct {
	CIK  int
	Name string
}

// CIK resolves a ticker to its central index key. See Registrant.
func (c *Client) CIK(ctx context.Context, ticker string) (int, error) {
	r, err := c.Registrant(ctx, ticker)
	return r.CIK, err
}

// Registrant resolves a ticker. The SEC's file is keyed by row number, so it is
// inverted once, and a dot is accepted for a class separator.
func (c *Client) Registrant(ctx context.Context, ticker string) (Registrant, error) {
	c.tickersOnce.Do(func() { c.tickers, c.tickersErr = c.loadTickers(ctx) })
	if c.tickersErr != nil {
		return Registrant{}, c.tickersErr
	}
	for _, k := range tickerSpellings(ticker) {
		if r, ok := c.tickers[k]; ok {
			return r, nil
		}
	}
	return Registrant{}, fmt.Errorf("%w: %s", ErrNotARegistrant, strings.ToUpper(ticker))
}

// tickerSpellings is the ticker as this program writes it and as the SEC writes
// it, both tried because the class separator is the only thing that differs.
func tickerSpellings(t string) []string {
	up := strings.ToUpper(strings.TrimSpace(t))
	out := []string{up}
	if strings.ContainsAny(up, ".-") {
		out = append(out, strings.ReplaceAll(up, ".", "-"), strings.ReplaceAll(up, "-", "."))
	}
	return out
}

func (c *Client) loadTickers(ctx context.Context) (map[string]Registrant, error) {
	var raw map[string]struct {
		CIK    int    `json:"cik_str"`
		Ticker string `json:"ticker"`
		Title  string `json:"title"`
	}
	if err := c.get(ctx, WWWBase+"/files/company_tickers.json", &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("edgar: company_tickers.json carried no rows")
	}
	out := make(map[string]Registrant, len(raw))
	for _, r := range raw {
		if r.Ticker != "" {
			out[strings.ToUpper(r.Ticker)] = Registrant{CIK: r.CIK, Name: r.Title}
		}
	}
	return out, nil
}

// Submissions reads one company's filing index. Only the `recent` block: it
// holds the last thousand filings, years past the latest 10-K and proxy.
func (c *Client) Submissions(ctx context.Context, cik int) (Submissions, error) {
	var s Submissions
	url := fmt.Sprintf("%s/submissions/CIK%010d.json", DataBase, cik)
	if err := c.get(ctx, url, &s); err != nil {
		return Submissions{}, err
	}
	s.CIK = cik
	return s, nil
}
