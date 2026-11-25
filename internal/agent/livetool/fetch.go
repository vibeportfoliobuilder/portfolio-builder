package livetool

// fetch.go - fetch_url. The URL must already be evidence: printed in one of
// this run's inputs, or recorded in the ledger by an earlier search.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/cache"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/httpx"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/edgar"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// ToolFetchURL is the tool's registered name, on both providers.
const ToolFetchURL = "fetch_url"

// fetchLimit is how much of a document is read at all, and bodyLimit how much
// of it reaches the model. A 10-K runs to megabytes and a turn does not.
const (
	fetchLimit = 8 << 20
	bodyLimit  = 24000
	// maxBodyLimit is the most a caller may ask for at once (a proxy's pay sections run to ~60,000).
	maxBodyLimit = 120000
)

// proxyAnchors are the sections of an SEC proxy (DEF 14A) the CEO filter needs; a clipped document
// lists where each starts, so the next call can ask for it.
var proxyAnchors = []string{"Compensation Discussion and Analysis", "Summary Compensation Table", "Pay Ratio",
	"Pay Versus Performance", "Grants of Plan-Based Awards", "Outstanding Equity Awards", "Security Ownership",
	"Director Compensation", "Related Person Transactions"}

// fetchPace is the gap between fetches. These are one-off reads of documents
// already named in the snapshot, not a crawl, so it only has to be polite.
const fetchPace = 500 * time.Millisecond

// ErrNoProvenance is a URL this run was never given. The point of the tool is
// that the address was already evidence; anything else is the model browsing.
var ErrNoProvenance = errors.New("livetool: that URL does not appear in this run's inputs or in " + LedgerFile)

// ErrNotWeb is a scheme this tool will not open. file:// would read the disk
// through a tool whose whole purpose is reaching OUTSIDE the snapshot.
var ErrNotWeb = errors.New("livetool: only http and https URLs can be fetched")

// FetchArgs is fetch_url's input schema. The jsonschema tag is comma-separated
// so every description here must stay comma-free.
type FetchArgs struct {
	URL string `json:"url" jsonschema:"required,description=The full URL to open. It must be one printed in an input you were given or returned by an earlier web_search - this tool will not open an address you invented."`
	// Find, Offset and Max read past the first clip of a long document (an SEC proxy).
	Find   string `json:"find,omitempty" jsonschema:"description=Optional text to start reading at (case-insensitive and the first match) such as Summary Compensation Table."`
	Offset int    `json:"offset,omitempty" jsonschema:"description=Optional character position to start reading at; a clipped document lists its sections with positions."`
	Max    int    `json:"max,omitempty" jsonschema:"description=Optional number of characters to return (default 24000 and at most 120000)."`
}

// Fetcher runs fetch_url. paths are the files whose text counts as provenance:
// this run's resolved inputs.
type Fetcher struct {
	http  *http.Client
	pacer *httpx.Pacer
	cache *cache.Cache
	dir   string
	paths []string
	now   time.Time
}

// NewFetcher binds a fetcher to the run's input files and the directory its
// citations land in. c may be nil, which costs a re-fetch rather than failing.
func NewFetcher(c *cache.Cache, dir string, paths []string, now time.Time) *Fetcher {
	return &Fetcher{
		http:  &http.Client{Timeout: 60 * time.Second},
		pacer: httpx.NewPacer(fetchPace),
		cache: c,
		dir:   dir,
		paths: paths,
		now:   now,
	}
}

// Description is what the model is told, including the one rule that decides
// whether a call will be answered at all.
func (f *Fetcher) Description() string {
	return "Open a URL that is ALREADY evidence in this run and return its text. " +
		"The address must be printed in one of your inputs - a filing section truncated to a " +
		"character budget prints the filing's own URL - or have been returned by an earlier " +
		"web_search. An address you invented is refused. What comes back is recorded in " +
		LedgerFile + " so a reader can check anything you cite from it."
}

// Fetch opens one URL, having first proved this run was given it.
func (f *Fetcher) Fetch(ctx context.Context, a FetchArgs) (string, error) {
	raw := strings.TrimSpace(a.URL)
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("livetool: %q is not a URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w: %q", ErrNotWeb, raw)
	}
	ok, err := f.known(raw)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNoProvenance, raw)
	}

	body, err := f.body(ctx, raw)
	if err != nil {
		return "", err
	}
	text := readable(body)
	shown, err := window(text, a)
	if err != nil {
		return "", err
	}
	// The text the reader is shown is kept beside the ledger, so a cited claim can be checked later.
	tf, err := saveText(f.dir, raw, shown)
	if err != nil {
		return "", err
	}

	// Recorded before it is returned, exactly as web_search records its hits.
	if _, err := Append(f.dir, f.now, []Source{{
		URL:      raw,
		Title:    title(body),
		Snippet:  clip(text, snippetLimit),
		TextFile: tf,
	}}); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s\n\nRecorded in %s (text in %s). Cite it by that URL.\n\n%s", raw, LedgerFile, tf, shown), nil
}

// window is the part of text the caller asked for (find, offset, max), clipped and marked, with the
// positions of the proxy sections when the whole text did not fit.
func window(text string, a FetchArgs) (string, error) {
	r := []rune(strings.TrimSpace(text))
	limit := bodyLimit
	if a.Max > 0 {
		limit = min(a.Max, maxBodyLimit)
	}
	start := max(0, a.Offset)
	if f := strings.TrimSpace(a.Find); f != "" {
		i := runeIndexFold(r, f, start)
		if i < 0 {
			return "", fmt.Errorf("livetool: %q does not occur in the document after character %d", f, start)
		}
		start = i
	}
	if start >= len(r) {
		return "", fmt.Errorf("livetool: offset %d is past the end of the document (%d characters)", start, len(r))
	}
	out := clipNoted(string(r[start:]), limit)
	if start > 0 {
		out = fmt.Sprintf("[from character %d of %d]\n\n", start, len(r)) + out
	}
	if len(r) > limit {
		var idx []string
		for _, h := range proxyAnchors {
			// The table of contents names a section first; the last match is usually the section itself.
			if i := lastRuneIndexFold(r, h); i >= 0 {
				idx = append(idx, fmt.Sprintf("%s @ %d", h, i))
			}
		}
		if len(idx) > 0 {
			out += "\n\n[sections: " + strings.Join(idx, "; ") + " - read one with --find or --offset]"
		}
	}
	return out, nil
}

func runeIndexFold(r []rune, sub string, from int) int {
	lower := strings.ToLower(string(r[min(from, len(r)):]))
	i := strings.Index(lower, strings.ToLower(sub))
	if i < 0 {
		return -1
	}
	return from + len([]rune(lower[:i]))
}

func lastRuneIndexFold(r []rune, sub string) int {
	lower := strings.ToLower(string(r))
	i := strings.LastIndex(lower, strings.ToLower(sub))
	if i < 0 {
		return -1
	}
	return len([]rune(lower[:i]))
}

// saveText writes the fetched text to <dir>/sources/<hash>.txt and returns that path relative to dir.
func saveText(dir, raw, text string) (string, error) {
	rel := filepath.Join("sources", store.SHA256Hex([]byte(raw))[:16]+".txt")
	if err := store.WriteFileAtomic(filepath.Join(dir, rel), []byte(text+"\n")); err != nil {
		return "", err
	}
	return rel, nil
}

// known reports whether this run was given the URL: recorded in the ledger by
// an earlier search, or printed in one of the files the run may read.
func (f *Fetcher) known(raw string) (bool, error) {
	l, err := ReadLedger(f.dir)
	if err != nil {
		return false, err
	}
	for _, s := range l.Sources {
		if s.URL == raw {
			return true, nil
		}
	}
	for _, p := range f.paths {
		b, err := os.ReadFile(p)
		if err != nil {
			// An input that cannot be read is not provenance, and is not this
			// tool's failure to report either.
			continue
		}
		if strings.Contains(string(b), raw) {
			return true, nil
		}
	}
	return false, nil
}

// body fetches the document, serving from and filling the cache. The key is the
// URL's hash rather than the URL, which can carry a query this run should not index.
func (f *Fetcher) body(ctx context.Context, raw string) ([]byte, error) {
	key := "fetch/" + store.SHA256Hex([]byte(raw))
	if f.cache != nil {
		if b, _, ok := f.cache.Get(key); ok {
			return b, nil
		}
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", fetchUserAgent())
		req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")
		return req, nil
	}
	resp, attempts, err := httpx.Do(ctx, f.http, f.pacer, nil, build)
	if err != nil {
		return nil, fmt.Errorf("livetool: fetching %s: %s", raw, httpx.WithAttempts(err.Error(), attempts))
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, fetchLimit))
	if err != nil {
		return nil, fmt.Errorf("livetool: reading %s: %w", raw, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("livetool: fetching %s: http %d", raw, resp.StatusCode)
	}
	if f.cache != nil {
		if _, err := f.cache.Put(key, raw, b, resp); err != nil {
			return nil, fmt.Errorf("livetool: caching %s: %w", raw, err)
		}
	}
	return b, nil
}

// fetchUserAgent is this project's own name, the same string every source sends.
func fetchUserAgent() string { return httpx.UserAgent }

var (
	// pageOnly is what a web page carries that is not its text: the head and the noscript
	// fallback. Spelled out per tag, as RE2 has no backreference.
	// An inline-XBRL filing (every SEC 10-K, 10-Q, 8-K) opens with a hidden ix:header of tagged facts;
	// read as text it is all the 400-character snippet ever held.
	pageOnly = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>|<noscript\b[^>]*>.*?</noscript>|<ix:header\b[^>]*>.*?</ix:header>|<script\b[^>]*>.*?</script>|<style\b[^>]*>.*?</style>`)
	anyTag   = regexp.MustCompile(`(?s)<[^>]*>`)
	titleTag = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
)

// readable turns a fetched page into text a turn can read: its head dropped, then edgar.Text,
// the one HTML-to-text rendering. Plain text passes through unchanged.
func readable(b []byte) string {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("%PDF")) {
		return pdfText(b)
	}
	s := string(b)
	if !strings.Contains(s, "<") {
		return strings.TrimSpace(s)
	}
	return edgar.Text([]byte(pageOnly.ReplaceAllString(s, " ")))
}

// title reads the document's <title>, which is what the ledger row is named by.
func title(b []byte) string {
	m := titleTag.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(anyTag.ReplaceAllString(string(m[1]), "")))
}
