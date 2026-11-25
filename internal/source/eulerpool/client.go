package eulerpool

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/httpx"
)

// Host is the only origin this package talks to.
const Host = "api.eulerpool.com"

// DefaultBase is the API root; a variable so a test can point it at httptest.
var DefaultBase = "https://" + Host + "/api/1/"

// KeyEnv names the credential. It travels in the Authorization header, never a URL.
const KeyEnv = "EULERPOOL_API_KEY"

// DefaultPace is the gap between requests; the vendor publishes no rate, 6 workers held 2026-11-06.
const DefaultPace = 120 * time.Millisecond

// TTLs per kind of route: how old a cached body may be and still answer.
const (
	TTLQuote     = 12 * time.Hour
	TTLNews      = 6 * time.Hour
	TTLFundament = 7 * 24 * time.Hour
	TTLMacro     = 12 * time.Hour
)

var (
	// ErrNoKey means KeyEnv is unset.
	ErrNoKey = errors.New("eulerpool: no API key (set " + KeyEnv + ")")
	// ErrUnauthorized is a 401 or 403: the key was refused or the tier does not cover the route.
	ErrUnauthorized = errors.New("eulerpool: the API key was refused")
	// ErrNotFound is a 404: the vendor has no such security or no such dataset for it.
	ErrNotFound = errors.New("eulerpool: not found")
)

// StatusError is any other refusal, carried with a clip of its body.
type StatusError = httpx.StatusError

// Client holds the transport, pacer, optional cache and key; safe for concurrent use.
type Client struct {
	HTTP    *http.Client
	Pacer   *httpx.Pacer
	Cache   Cache
	Now     func() time.Time
	Backoff []time.Duration
	Base    string

	key      string
	requests atomic.Int64
}

// NewClient builds a client with the key from the environment and no cache.
func NewClient() (*Client, error) {
	key := strings.TrimSpace(os.Getenv(KeyEnv))
	if key == "" {
		return nil, ErrNoKey
	}
	return &Client{HTTP: &http.Client{Timeout: 90 * time.Second}, Pacer: httpx.NewPacer(DefaultPace), key: key}, nil
}

// NewClientWithKey builds a client around an explicit key; tests use it.
func NewClientWithKey(key string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 90 * time.Second}, Pacer: httpx.NewPacer(DefaultPace), key: key}
}

// Requests is how many requests reached the network (cache hits excluded).
func (c *Client) Requests() int64 { return c.requests.Load() }

func (c *Client) base() string { return cmp.Or(c.Base, DefaultBase) }

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// cacheKey is the route and its sorted query; it never contains the credential.
func cacheKey(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// get fetches one route relative to the base, serving from the cache when fresh within ttl.
func (c *Client) get(ctx context.Context, path string, q url.Values, ttl time.Duration) ([]byte, error) {
	key := cacheKey(path, q)
	if c.Cache != nil {
		if b, at, ok := c.Cache.Get(key); ok && c.now().Sub(at) < ttl {
			if string(b) == notFoundMarker {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			return b, nil
		}
	}
	u := c.base() + key
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", httpx.UserAgent)
		return req, nil
	}
	c.requests.Add(1)
	return httpx.Exchange(ctx, c.HTTP, c.Pacer, c.Backoff, "eulerpool", path, 64<<20, build, func(resp *http.Response, raw []byte) ([]byte, error) {
		switch resp.StatusCode {
		case http.StatusOK:
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("%w: %s: http %d", ErrUnauthorized, path, resp.StatusCode)
		case http.StatusNotFound:
			if c.Cache != nil {
				_ = c.Cache.Put(key, []byte(notFoundMarker), c.now())
			}
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		default:
			return nil, &StatusError{Source: "eulerpool", Status: resp.StatusCode, Path: path, Body: string(raw)}
		}
		if c.Cache != nil {
			// A failed cache write costs a re-download, not the answer.
			_ = c.Cache.Put(key, raw, c.now())
		}
		return raw, nil
	})
}

// notFoundMarker is what the cache holds for a 404, so an absent dataset is not re-asked within its TTL.
const notFoundMarker = "eulerpool:404"

// seg escapes one path segment (an ISIN, a ticker such as VOW3.DE).
func seg(s string) string { return url.PathEscape(strings.TrimSpace(s)) }
