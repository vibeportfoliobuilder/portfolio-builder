package edgar

// document.go - fetching a filing DOCUMENT: a modern inline-XBRL 10-K is 8.5 MB
// of markup for 355 KB of text, so the body is read to a ceiling.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/httpx"
)

// MaxDocumentBytes is the most one filing document may be, about four times the
// largest measured: an 8.5 MB inline-XBRL 10-K.
const MaxDocumentBytes = 32 << 20

// Document reads one filing document by its URL. The bytes are verbatim, so a
// stored fixture records the SEC's document rather than this package's stripper.
func (c *Client) Document(ctx context.Context, url string) ([]byte, error) {
	return c.fetch(ctx, url)
}

// fetch is the one request shape this package makes: paced, with the required
// header, read to a ceiling. Shared with get, which adds the JSON decode.
func (c *Client) fetch(ctx context.Context, url string) ([]byte, error) {
	b, _, err := c.fetchStatus(ctx, url)
	return b, err
}

// fetchStatus is fetch with the status code, for the one caller that has to
// tell "this filer never tagged that" from "the SEC refused us".
func (c *Client) fetchStatus(ctx context.Context, url string) ([]byte, int, error) {
	resp, attempts, err := httpx.Do(ctx, c.HC, c.Pacer, c.Backoff, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		// The SEC requires this header (see defaultUserAgent). Accept-Encoding stays
		// unset so net/http both asks for gzip and decodes it; set by hand, it would not.
		req.Header.Set("User-Agent", c.UserAgent)
		return req, nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("edgar: %s: %w", httpx.WithAttempts(url, attempts), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("edgar: %s: http %d", url, resp.StatusCode)
	}
	b, err := httpx.ReadCapped(resp.Body, MaxDocumentBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("edgar: %s: %w", url, err)
	}
	return b, resp.StatusCode, nil
}
