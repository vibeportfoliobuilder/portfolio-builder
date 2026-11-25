package livetool

// news.go - news_search walks the configured feeds (Eulerpool for US names, the broker's
// for the rest) and writes the ledger before answering; read_news_article stores NOTHING.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/docfmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// The two news tools, on both providers.
const (
	ToolNewsSearch      = "news_search"
	ToolReadNewsArticle = "read_news_article"
)

// NewsLedgerFile is the archive of what was PUBLISHED. Bodies never go in it.
const NewsLedgerFile = "news.yaml"

// The pipes a row arrived down, which are not the newsrooms that wrote it:
// corroboration keys on Publisher, because one wire item comes down both.
const (
	vendorEulerpool = "eulerpool"
)

// eulerpoolItems is how many items one Eulerpool walk asks for (the feed serves about 20).
const eulerpoolItems = 50

// newsBodyLimit is how much of an article reaches the model in one call.
const newsBodyLimit = 20000

// ErrArticleNotListed is an article id this run never recorded. The ledger is
// the provenance, which is what makes reading an unstorable body defensible.
var ErrArticleNotListed = errors.New("livetool: that article is not in " + NewsLedgerFile)

// ErrNoFeed is a walk with no pipe to read. It is what a run with neither a
// broker session nor an Eulerpool key gets, and it is reportable rather than fatal.
var ErrNoFeed = errors.New("livetool: no news feed is configured")

// NewsRecord is one published item. URL is empty where a feed has no article address,
// and an invented one would be worse than none.
type NewsRecord struct {
	Vendor      string   `yaml:"vendor"`
	ArticleID   string   `yaml:"article_id"`
	Publisher   string   `yaml:"publisher,omitempty"`
	Headline    string   `yaml:"headline,omitempty"`
	PublishedAt string   `yaml:"published_at,omitempty"`
	RetrievedAt string   `yaml:"retrieved_at,omitempty"`
	URL         string   `yaml:"url,omitempty"`
	Related     []string `yaml:"related_tickers,omitempty"`
}

// key is what a record is deduped on: the pipe plus the vendor's own id.
func (r NewsRecord) key() string { return r.Vendor + "/" + r.ArticleID }

// NewsLedger is the on-disk shape of news.yaml.
type NewsLedger struct {
	Items []NewsRecord `yaml:"items,omitempty"`
}

// ReadNewsLedger reads dir's news.yaml. A missing file is an empty ledger.
func ReadNewsLedger(dir string) (NewsLedger, error) {
	return readYAML[NewsLedger](dir, NewsLedgerFile)
}

// AppendNews merges rows into dir's news.yaml and reports how many were new.
// First sighting wins on retrieved_at, so a re-run changes no bytes.
func AppendNews(dir string, now time.Time, rows []NewsRecord) (int, error) {
	was, err := ReadNewsLedger(dir)
	if err != nil {
		return 0, err
	}
	out, added, err := MergeNews(was, now, rows)
	if err != nil {
		return 0, err
	}
	if err := writeYAML(dir, NewsLedgerFile, out); err != nil {
		return 0, err
	}
	return added, nil
}

// MergeNews folds rows into a ledger and reports how many were new. The seam a
// caller that writes through the snapshot store uses instead of AppendNews.
func MergeNews(old NewsLedger, now time.Time, rows []NewsRecord) (NewsLedger, int, error) {
	by := make(map[string]NewsRecord, len(old.Items)+len(rows))
	for _, r := range old.Items {
		by[r.key()] = r
	}
	before := len(by)
	stamp := docfmt.FormatDate(now)
	for _, r := range rows {
		if strings.TrimSpace(r.ArticleID) == "" {
			continue
		}
		if _, seen := by[r.key()]; seen {
			continue
		}
		if r.RetrievedAt == "" {
			r.RetrievedAt = stamp
		}
		by[r.key()] = r
	}
	out := NewsLedger{Items: make([]NewsRecord, 0, len(by))}
	for _, r := range by {
		out.Items = append(out.Items, r)
	}
	// Newest first, then by key so the order is total and the file is stable.
	slices.SortFunc(out.Items, func(a, b NewsRecord) int {
		if a.PublishedAt != b.PublishedAt {
			return strings.Compare(b.PublishedAt, a.PublishedAt)
		}
		return strings.Compare(a.key(), b.key())
	})
	for _, was := range old.Items {
		if _, still := by[was.key()]; !still {
			return NewsLedger{}, 0, fmt.Errorf("%w: %s", ErrLedgerShrank, was.key())
		}
	}
	return out, len(by) - before, nil
}

// NewsArgs is news_search's input schema; it takes nothing, because the
// contract this run is about is fixed when the tool is built.
type NewsArgs struct {
	Pages int `json:"pages,omitempty" jsonschema:"description=How many pages of history to walk. One page is the most recent 50 items; defaults to 1 and is capped."`
}

// ReadArticleArgs is read_news_article's input schema.
type ReadArticleArgs struct {
	ArticleID string `json:"article_id" jsonschema:"required,description=The article_id of an item news_search already listed. An id that is not in the ledger is refused."`
}

// Newsroom runs the two news tools against one contract, over every feed it was
// given.
type Newsroom struct {
	feeds    Feeds
	brokerID string
	ticker   string
	dir      string
	now      time.Time
}

// NewNewsroom binds the news tools to one contract and the directory its ledger
// lives in. The broker's feed alone (nil for a US name); see WithEulerpool for the other.
func NewNewsroom(f BrokerFeed, brokerID string, ticker, dir string, now time.Time) *Newsroom {
	return &Newsroom{feeds: Feeds{Broker: f}, brokerID: brokerID, ticker: ticker, dir: dir, now: now}
}

// WithEulerpool adds Eulerpool's company feed, keyed by the ticker or ISIN the newsroom was
// built with. A nil client leaves the newsroom on the broker's feed alone.
func (n *Newsroom) WithEulerpool(c *eulerpool.Client) *Newsroom {
	n.feeds.Eulerpool = c
	return n
}

// SearchDescription is what the model is told news_search does.
func (n *Newsroom) SearchDescription() string {
	return "List recent news about " + n.ticker + " from " + n.feedNames() + ". Every item is written " +
		"to " + NewsLedgerFile + " before you see it and each carries an article_id you can pass to " +
		ToolReadNewsArticle + ". The list is the headline record; it is deliberately not the text. " +
		"Two rows can be one story down two pipes, so corroborate on the PUBLISHER and never on the vendor."
}

// feedNames says which pipes this run actually has, so the model is not told
// about a feed whose credential was absent.
func (n *Newsroom) feedNames() string {
	switch {
	case n.feeds.Broker != nil && n.feeds.Eulerpool != nil:
		return "the broker's feed and Eulerpool's"
	case n.feeds.Eulerpool != nil:
		return "Eulerpool"
	default:
		return "the broker's feed"
	}
}

// ReadDescription says what read_news_article does AND what it refuses.
func (n *Newsroom) ReadDescription() string {
	return "Read one article's full text by the article_id " + ToolNewsSearch + " listed. " +
		"Only a broker row carries its body here; an Eulerpool row is an address, and this tool " +
		"hands you that address to open with " + ToolFetchURL + ". " +
		"The text is NOT stored: the broker feed is licensed to this account to read and not to " +
		"this repository to republish, so quote it sparingly and cite the article_id and publisher " +
		"rather than pasting the body into your output."
}

// Search walks every feed in one pass, records every item, and returns the
// headline list.
func (n *Newsroom) Search(ctx context.Context, a NewsArgs) (string, error) {
	rows, notes, err := n.feeds.Walk(ctx, n.brokerID, n.ticker, a.Pages, n.now)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		out := fmt.Sprintf("No news listed for %s. Record the gap rather than inferring around it.", n.ticker)
		if len(notes) > 0 {
			out += "\n" + strings.Join(notes, "\n")
		}
		return out, nil
	}
	// Recorded before the headlines are returned, like every other live tool.
	if _, err := AppendNews(n.dir, n.now, rows); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d items for %s, all recorded in %s.\n", len(rows), n.ticker, NewsLedgerFile)
	b.WriteString("Corroborate on the PUBLISHER: one wire item reaching two vendors is one witness.\n")
	for _, note := range notes {
		b.WriteString(note + "\n")
	}
	b.WriteString("\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "- %s  [%s]  %s\n  id: %s (%s)\n",
			newsStamp(r.PublishedAt), r.Publisher, r.Headline, r.ArticleID, r.Vendor)
	}
	return b.String(), nil
}

// newsStamp shortens an RFC3339 instant to the minute for the listing. A row
// whose date did not parse is shown as it was recorded rather than as a zero.
func newsStamp(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// Read opens one article. It records NOTHING, and is the single exception to
// the rule that a tool writes what it found: see ReadDescription for why.
func (n *Newsroom) Read(ctx context.Context, a ReadArticleArgs) (string, error) {
	id := strings.TrimSpace(a.ArticleID)
	l, err := ReadNewsLedger(n.dir)
	if err != nil {
		return "", err
	}
	// The ledger IS the provenance. Without this check the exception above would
	// let the model read anything the account is entitled to and cite none of it.
	i := slices.IndexFunc(l.Items, func(r NewsRecord) bool { return r.ArticleID == id })
	if i < 0 {
		return "", fmt.Errorf("%w: %s", ErrArticleNotListed, id)
	}
	// Eulerpool serves an ADDRESS and no body route, so this tool hands the address
	// over rather than pretending to a text it cannot fetch.
	if row := l.Items[i]; row.Vendor == vendorEulerpool {
		return fmt.Sprintf("%s [%s] is an Eulerpool row, which carries an address and no body: %s\n"+
			"Open it with %s, which records what it found, or cite it by id and publisher and say the body was not read.",
			id, row.Publisher, row.URL, ToolFetchURL), nil
	}
	if n.feeds.Broker == nil {
		return "", fmt.Errorf("livetool: %s is a broker row and this run has no broker feed", id)
	}
	return n.feeds.Broker.Article(ctx, id)
}
