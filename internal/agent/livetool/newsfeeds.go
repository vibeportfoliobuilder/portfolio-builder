package livetool

// newsfeeds.go - the one walk: the broker's feed (non-US names, when a connector supplies one)
// and Eulerpool's (US names), each optional.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// maxNewsPages bounds one walk. A heavily covered name runs about fifty items a
// day so this is weeks of history for most and a day or two for the busiest.
const maxNewsPages = 5

// BrokerFeed is a broker's own news feed, supplied by a broker connector (RegisterBrokerFeed).
type BrokerFeed interface {
	// Search pages the feed for one instrument id, at most pages pages; rows that paged in
	// before an error come back with it. Each row names its own Vendor.
	Search(ctx context.Context, id string, pages int) ([]NewsRecord, error)
	// Article is one article's text for the model to read (never stored), or a sentence saying
	// why there is none to read.
	Article(ctx context.Context, articleID string) (string, error)
}

// Feeds are the pipes one news walk reads. Either may be nil: a missing
// credential drops that feed and is reported, never a failed run.
type Feeds struct {
	Broker    BrokerFeed
	Eulerpool *eulerpool.Client
}

// Any reports whether there is anything to walk at all.
func (f Feeds) Any() bool { return f.Broker != nil || f.Eulerpool != nil }

// Walk reads every configured feed for one contract and returns ledger rows; err only
// when EVERY feed failed, a partial failure being a note, as half the news beats none.
func (f Feeds) Walk(ctx context.Context, brokerID string, symbol string, pages int, now time.Time) ([]NewsRecord, []string, error) {
	if !f.Any() {
		return nil, nil, ErrNoFeed
	}
	var rows []NewsRecord
	var notes []string
	var tried, failed int

	if f.Broker != nil && brokerID != "" {
		tried++
		got, err := f.Broker.Search(ctx, brokerID, min(max(pages, 1), maxNewsPages))
		if err != nil {
			failed++
			notes = append(notes, "broker: the broker's news feed could not be read ("+err.Error()+")")
		}
		rows = append(rows, got...)
	}
	if f.Eulerpool != nil {
		tried++
		got, err := f.walkEulerpool(ctx, symbol)
		if err != nil {
			failed++
			notes = append(notes, "eulerpool: the company-news route could not be read ("+err.Error()+")")
		}
		rows = append(rows, got...)
	}
	if tried == 0 {
		return nil, notes, nil
	}
	if failed == tried {
		return nil, notes, fmt.Errorf("livetool: every news feed failed: %s", notes[0])
	}
	return rows, notes, nil
}

// walkEulerpool reads the vendor's company feed for a ticker or ISIN. The article id is
// a digest of the URL, since the feed carries none of its own.
func (f Feeds) walkEulerpool(ctx context.Context, symbol string) ([]NewsRecord, error) {
	if symbol == "" {
		return nil, nil
	}
	items, err := f.Eulerpool.News(ctx, symbol, eulerpoolItems)
	if err != nil {
		return nil, err
	}
	out := make([]NewsRecord, 0, len(items))
	for _, it := range items {
		sum := sha256.Sum256([]byte(it.URL))
		out = append(out, NewsRecord{
			Vendor:      vendorEulerpool,
			ArticleID:   "ep-" + hex.EncodeToString(sum[:6]),
			Publisher:   it.Source,
			Headline:    it.Headline,
			PublishedAt: it.Date,
			URL:         it.URL,
		})
	}
	return out, nil
}
