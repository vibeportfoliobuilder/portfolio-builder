package livetool

// set.go - the outward tool surface for one pb call: search, fetch and the news pair.

import (
	"fmt"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/cache"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/tavily"
)

// Config is what one call needs to build its outward tools.
type Config struct {
	// Dir is the writable folder where the ledgers (sources.yaml, news.yaml) land.
	Dir string
	// CacheDir is the raw-response cache root, outside the repository. Empty means uncached.
	CacheDir string
	// InputPaths are files whose URLs fetch_url treats as provenance.
	InputPaths []string
	// Now is the instant threaded through the call.
	Now time.Time
	// BrokerID and Ticker name the instrument the news tools are about (BrokerID is the active
	// broker's own id for it, a number or a UUID); Symbol is the Eulerpool ticker or ISIN. US
	// routes news to Eulerpool, anything else to the broker's feed.
	BrokerID string
	Ticker   string
	Symbol   string
	US       bool
}

// BrokerFeedOpener opens a broker's news feed from the environment (its session, say); blobs is
// the raw cache. A broker connector registers one with RegisterBrokerFeed.
type BrokerFeedOpener func(blobs *cache.Cache) (BrokerFeed, error)

var brokerFeeds = map[string]BrokerFeedOpener{}

// RegisterBrokerFeed installs the news feed NewSet uses for non-US names while the broker pack
// named key is active (pack.Broker.Key); several connectors can each register their own.
func RegisterBrokerFeed(key string, f BrokerFeedOpener) {
	brokerFeeds[strings.ToLower(strings.TrimSpace(key))] = f
}

func openBrokerFeed() BrokerFeedOpener {
	return brokerFeeds[strings.ToLower(pack.Active().Broker.Key)]
}

// Set is one call's outward tools. News is nil when no feed can serve the name.
type Set struct {
	Search *Searcher
	Fetch  *Fetcher
	News   *Newsroom
}

// OpenCache opens the raw cache at dir, or returns nil for an empty dir.
func OpenCache(dir string) (*cache.Cache, error) {
	if dir == "" {
		return nil, nil
	}
	c, err := cache.OpenCache(dir)
	if err != nil {
		return nil, fmt.Errorf("livetool: opening the cache: %w", err)
	}
	c.Persist = true
	return c, nil
}

// NewSet builds the tools. A missing Tavily key leaves Search nil; a missing feed
// credential drops the news pair rather than failing.
func NewSet(cfg Config) (*Set, error) {
	blobs, err := OpenCache(cfg.CacheDir)
	if err != nil {
		return nil, err
	}
	s := &Set{Fetch: NewFetcher(blobs, cfg.Dir, cfg.InputPaths, cfg.Now)}
	if client, err := tavily.NewClient(); err == nil {
		client.Cache = blobs
		s.Search = NewSearcher(client, cfg.Dir, cfg.Now)
	}
	if cfg.Ticker == "" {
		return s, nil
	}
	if cfg.US {
		ep, err := eulerpool.NewClient()
		if err != nil {
			return s, nil //nolint:nilerr // no key is a missing tool, not an error
		}
		if d, err := eulerpool.DefaultCacheDir(); err == nil {
			ep.Cache = eulerpool.NewDiskCache(d)
		}
		sym := cfg.Symbol
		if sym == "" {
			sym = cfg.Ticker
		}
		s.News = NewNewsroom(nil, "", sym, cfg.Dir, cfg.Now).WithEulerpool(ep)
		return s, nil
	}
	open := openBrokerFeed()
	if open == nil || cfg.BrokerID == "" {
		return s, nil
	}
	f, err := open(blobs)
	if err != nil {
		return s, nil //nolint:nilerr // no broker session is a missing tool, not an error
	}
	s.News = NewNewsroom(f, cfg.BrokerID, cfg.Ticker, cfg.Dir, cfg.Now)
	return s, nil
}
