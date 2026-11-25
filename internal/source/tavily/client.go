package tavily

// client.go - the transport: one origin, one route (POST /search), and a key
// that travels in the Authorization header so no URL ever carries it.

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/cache"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/httpx"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// Host is the only origin this package talks to.
const Host = "api.tavily.com"

// DefaultBase is that origin as a URL. A variable, not a constant, so a test
// can point the client at an httptest server.
var DefaultBase = "https://" + Host

// KeyEnv is where the credential comes from. Screaming-snake like every other
// credential here; data/secrets/keys stays an operator convenience to source into a shell.
const KeyEnv = "TAVILY_API_KEY"

// DefaultPace is the 100 a minute Tavily publishes for a development key, plus
// 15%. Not provoked: every search spends a credit, read from /usage 2026-10-04.
const DefaultPace = 690 * time.Millisecond

// userAgent is this project's own name, the same string every other source sends.
const userAgent = httpx.UserAgent

// The failures a caller has to tell apart, because each calls for something else:
// fix the environment, fix the credential, or retry.
var (
	// ErrNoKey means KeyEnv is unset. Distinguishable from a rejected key, which
	// is a 401 the operator cannot fix by exporting anything.
	ErrNoKey = errors.New("tavily: no API key (set " + KeyEnv + ")")
	// ErrUnauthorized is the origin refusing the key. Measured 2026-09-24: 401
	// with {"detail":{"error":...}}, identical for a bad key and a missing one.
	ErrUnauthorized = errors.New("tavily: the API key was refused")
)

// StatusError is any other refusal, carried with enough of the body to be
// diagnosable without a packet capture.
type StatusError = httpx.StatusError

// Client holds the transport, the pacer and the key. Cache is optional: with
// one set a repeated query costs nothing and every body becomes a fixture.
type Client struct {
	HTTP  *http.Client
	Pacer *httpx.Pacer
	Cache *cache.Cache

	// MaxAge overrides DefaultMaxAge, how old a cached answer may be; negative never expires.
	// Now is the instant that age is measured against, injectable for a test.
	MaxAge time.Duration
	Now    func() time.Time

	// Backoff is the retry schedule; nil is httpx.DefaultBackoff().
	Backoff []time.Duration

	// Base is the origin. Only a test sets it.
	Base string

	key string
}

// NewClient builds a client with the key from the environment. An absent key is
// reported here rather than as a 401 the caller would have to interpret.
func NewClient() (*Client, error) {
	key := strings.TrimSpace(os.Getenv(KeyEnv))
	if key == "" {
		return nil, ErrNoKey
	}
	return &Client{
		HTTP:  &http.Client{Timeout: 30 * time.Second},
		Pacer: httpx.NewPacer(DefaultPace),
		key:   key,
	}, nil
}

func (c *Client) base() string { return cmp.Or(c.Base, DefaultBase) }

// DefaultMaxAge is how old a cached answer may be and still serve: twenty hours, as for every
// other client. A key carries no date, so this bound is what re-asks the web a day later.
const DefaultMaxAge = 20 * time.Hour

// freshEnough reports whether a cached answer fetched at t may stand in for a new request.
func (c *Client) freshEnough(t time.Time) bool {
	return cache.FreshEnough(t, func() time.Time { return core.NowOr(c.Now) }, c.MaxAge, DefaultMaxAge)
}

// post sends one JSON body to path and returns the raw response, serving from
// and filling the cache under key. The key is the caller's: it must not be a URL.
func (c *Client) post(ctx context.Context, path, key string, body []byte) ([]byte, error) {
	if c.Cache != nil {
		if b, e, ok := c.Cache.Get(key); ok && c.freshEnough(e.FetchedAt) {
			return b, nil
		}
	}
	u := c.base() + path
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", userAgent)
		return req, nil
	}
	return httpx.Exchange(ctx, c.HTTP, c.Pacer, c.Backoff, "tavily", path, 16<<20, build, func(resp *http.Response, raw []byte) ([]byte, error) {
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, ErrUnauthorized
		case resp.StatusCode != http.StatusOK:
			return nil, &StatusError{Source: "tavily", Status: resp.StatusCode, Body: string(raw)}
		}
		if c.Cache != nil {
			// The URL is the constant route and carries no credential, so storing it
			// beside the body cannot leak the key the way a query-string key would.
			if _, err := c.Cache.Put(key, u, raw, resp); err != nil {
				return nil, fmt.Errorf("tavily: caching %s: %w", key, err)
			}
		}
		return raw, nil
	})
}

// cacheKey addresses one request by what was ASKED, never by the URL: every
// query posts to the same route, so a URL-derived key would collide on all of them.
func cacheKey(body []byte) string {
	return "tavily/search/" + store.SHA256Hex(body)
}

// marshal encodes a request in its struct's fixed field order, so the same query
// always hashes to the same cache key.
func marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("tavily: encoding the request: %w", err)
	}
	return b, nil
}
