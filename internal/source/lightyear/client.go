package lightyear

// client.go - the transport: one origin, one page shape, no credential.
// See doc.go for why the screener is read off the page rather than an API.

import (
	"cmp"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/cache"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/httpx"
)

// Host is the only origin this package talks to.
const Host = "lightyear.com"

// DefaultBase is that origin as a URL. A variable, not a constant, so a test
// can point the client at an httptest server.
var DefaultBase = "https://" + Host

// DefaultPace is 245ms plus 15%: at 210ms the pages came five times slower.
// 1,071 pages held over five minutes, measured 2026-10-04.
const DefaultPace = 280 * time.Millisecond

// userAgent is the project's own name, as every other source here sends.
const userAgent = httpx.UserAgent

// DefaultMaxAge is how old a cached page may be and still answer. A broker's
// listed set moves in weeks, and internal/cache has no TTL of its own.
const DefaultMaxAge = 20 * time.Hour

// Client is this package's one thing to hold: an HTTP client and a pacer.
type Client struct {
	HTTP  *http.Client
	Pacer *httpx.Pacer

	// Cache is optional and opt-in PER CALL: a route names the key it wants, and
	// an empty key means do not cache.
	Cache *cache.Cache

	// MaxAge overrides DefaultMaxAge. Zero is the default; a NEGATIVE value means
	// no expiry at all, which only a replay should set.
	MaxAge time.Duration

	// Now is the instant a cached body's age is measured against. Injectable so
	// an expiry can be tested without waiting for one.
	Now func() time.Time

	// Backoff is this client's retry schedule: the wait before each retry, and by
	// its length how many there are. nil is httpx.DefaultBackoff().
	Backoff []time.Duration

	// Base is the origin. Only a test sets it.
	Base string
}

// NewClient builds a client with the shared transport and this package's pace.
func NewClient() *Client {
	return &Client{
		HTTP:  &http.Client{Timeout: 60 * time.Second},
		Pacer: httpx.NewPacer(DefaultPace),
	}
}

func (c *Client) base() string { return cmp.Or(c.Base, DefaultBase) }

func (c *Client) now() time.Time { return core.NowOr(c.Now) }

// freshEnough reports whether a cached body may stand in for this request.
func (c *Client) freshEnough(t time.Time) bool {
	return cache.FreshEnough(t, c.now, c.MaxAge, DefaultMaxAge)
}

// cacheKey names one page of the walk. It carries no date - the listed set is
// not a daily fact - so DefaultMaxAge is what expires it.
func cacheKey(page int) string {
	return "lightyear/explore/page-" + strconv.Itoa(page) + ".html"
}

// StatusError is a non-200 response, carried with enough of the body to be
// diagnosable without a packet capture.
type StatusError = httpx.StatusError

// get sends one GET and returns a page to decode. An empty cacheKey means do not
// cache, which is the default so a later route does not acquire one by accident.
func (c *Client) get(ctx context.Context, path, cacheKey string, opts ...httpx.Option) ([]byte, error) {
	if cacheKey != "" && c.Cache != nil {
		if b, e, ok := c.Cache.Get(cacheKey); ok && len(b) > 0 && c.freshEnough(e.FetchedAt) {
			return b, nil
		}
	}
	u := c.base() + path
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/html")
		req.Header.Set("User-Agent", userAgent)
		return req, nil
	}
	return httpx.Exchange(ctx, c.HTTP, c.Pacer, c.Backoff, "lightyear", path, 16<<20, build, func(resp *http.Response, raw []byte) ([]byte, error) {
		if resp.StatusCode != http.StatusOK {
			return nil, &StatusError{Source: "lightyear", Status: resp.StatusCode, Path: path, Body: string(raw)}
		}
		if cacheKey != "" && c.Cache != nil {
			// A cache write that fails costs a re-download and nothing else, which is
			// not worth failing the walk over.
			_, _ = c.Cache.Put(cacheKey, u, raw, resp)
		}
		return raw, nil
	}, opts...)
}
