package pbcli

// tools.go - the agent tools: pb search, pb fetch-url, pb news search, pb news read (pb news list
// is in newslist.go).

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/agent/livetool"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

// ledgerDir is where a tool's citations land: the ticker's folder, else data/research.
func (e *env) ledgerDir(ticker string) string {
	if ticker != "" {
		// A fund's citations go with its data under data/etfs.
		t := target{Ticker: dirName(ticker), ID: ticker}
		if w, err := e.readWatchlist(); err == nil {
			for i := range w.Items {
				if strings.EqualFold(w.Items[i].Ticker, ticker) {
					t.Item = &w.Items[i]
				}
			}
		}
		if e.isFund(t) {
			return e.path("etfs", dirName(ticker))
		}
		return e.path("stocks", dirName(ticker))
	}
	return e.path("research")
}

// dataFiles are every YAML and Markdown file under data/: what fetch-url accepts as provenance.
func (e *env) dataFiles() []string {
	var out []string
	_ = filepath.WalkDir(e.data, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(p); ext == ".yaml" || ext == ".md" {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func searchCmd(e *env) *cobra.Command {
	var ticker string
	var n int
	c := &cobra.Command{
		Use:   "search <query>",
		Short: "Web search (Tavily); every hit is recorded in sources.yaml before it is printed",
		Long: `Searches the web for judgement the data files cannot give (industry structure, a CEO's record).
Hits are appended to data/stocks/<T>/sources.yaml with --ticker (data/etfs/<T>/ for a fund), else data/research/sources.yaml,
and printed as Markdown. Cite a hit by its URL. Needs TAVILY_API_KEY.
Example:
  pb search "Coca-Cola unit case volume growth 2026" --ticker KO`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := livetool.NewSet(livetool.Config{Dir: e.ledgerDir(ticker), CacheDir: cacheRoot(), Now: e.now()})
			if err != nil {
				return err
			}
			if s.Search == nil {
				return fmt.Errorf("no TAVILY_API_KEY in the environment")
			}
			out, err := s.Search.Search(cmd.Context(), livetool.SearchArgs{Query: strings.Join(args, " "), MaxResults: n})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(e.out, out)
			return err
		},
	}
	c.Flags().StringVar(&ticker, "ticker", "", "record the hits under this ticker")
	c.Flags().IntVar(&n, "max", 5, "how many results")
	return c
}

// cacheRoot is the raw cache's root, or "" when it could not be opened.
func cacheRoot() string {
	d, err := rawCacheDir()
	if err != nil {
		return ""
	}
	return d
}

func fetchURLCmd(e *env) *cobra.Command {
	var ticker, find string
	var offset, maxChars int
	c := &cobra.Command{
		Use:   "fetch-url <url>",
		Short: "Open a URL already in evidence (in a data/ file or an earlier search) and print its text",
		Long: `Opens one http(s) URL and prints its readable text (clipped). The URL must already appear in a
file under data/ (a filing URL in edgar.yaml, a KID URL in broker.yaml, a hit in sources.yaml);
an invented address is refused. What was read is recorded in sources.yaml (--ticker as for search).
The text is clipped at 24,000 characters; a longer document ends with the positions of an SEC proxy's
sections (CD&A, Summary Compensation Table, pay ratio ...), and --find, --offset and --max (up to
120,000) read further.
Examples:
  pb fetch-url https://www.sec.gov/Archives/edgar/data/.../ko-20251231.htm --ticker KO
  pb fetch-url <DEF 14A url> --ticker KO --find "Summary Compensation Table" --max 40000`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := livetool.NewSet(livetool.Config{Dir: e.ledgerDir(ticker), CacheDir: cacheRoot(), InputPaths: e.dataFiles(), Now: e.now()})
			if err != nil {
				return err
			}
			out, err := s.Fetch.Fetch(cmd.Context(), livetool.FetchArgs{URL: args[0], Find: find, Offset: offset, Max: maxChars})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(e.out, out)
			return err
		},
	}
	c.Flags().StringVar(&ticker, "ticker", "", "record under this ticker")
	c.Flags().StringVar(&find, "find", "", "start at the first match of this text (case-insensitive)")
	c.Flags().IntVar(&offset, "offset", 0, "start at this character")
	c.Flags().IntVar(&maxChars, "max", 0, "characters to print (default 24000, at most 120000)")
	return c
}

func newsCmd(e *env) *cobra.Command {
	c := &cobra.Command{
		Use:   "news",
		Short: "Company news: Eulerpool for US names, the broker's feed (if a connector supplies one) for non-US names",
		Long: `news search lists recent headlines for one name and records them in data/stocks/<T>/news.yaml;
news list prints the saved headlines one line each, with --dedupe folding duplicates and dropping noise;
news read opens one listed article (broker rows may carry a body; Eulerpool rows are an address to open
with pb fetch-url). A name is US when its ISIN starts with US (or its country is US). US news needs
EULERPOOL_API_KEY; non-US news needs a broker connector with a news feed, else the saved headlines
in news.yaml are shown.`,
	}
	var pages int
	search := &cobra.Command{
		Use:   "search <TICKER>",
		Short: "List and record recent news for one name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := e.newsroom(cmd, args[0])
			var out string
			if err == nil {
				out, err = s.Search(cmd.Context(), livetool.NewsArgs{Pages: pages})
			}
			if err != nil {
				if !noBrokerSession(err) {
					return err
				}
				// No live broker feed: the headlines a connector's sync saved are the source, so they are shown.
				return e.savedNews(args[0], err)
			}
			_, err = fmt.Fprintln(e.out, out)
			return err
		},
	}
	search.Flags().IntVar(&pages, "pages", 1, "broker feed pages (max 5)")
	read := &cobra.Command{
		Use:   "read <TICKER> <article_id>",
		Short: "Read one article listed by news search (not stored; cite id and publisher)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := e.newsroom(cmd, args[0])
			if err != nil {
				return err
			}
			out, err := s.Read(cmd.Context(), livetool.ReadArticleArgs{ArticleID: args[1]})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(e.out, out)
			return err
		},
	}
	c.AddCommand(search, read, newsListCmd(e))
	return c
}

// newsroom routes one name: Eulerpool for a US name, the broker's feed (needs the broker's id) otherwise.
func (e *env) newsroom(cmd *cobra.Command, arg string) (*livetool.Newsroom, error) {
	ts, err := e.targets([]string{arg}, false, "")
	if err != nil {
		return nil, err
	}
	t := ts[0]
	var d eulerpool.Dossier
	_, _ = e.readYAML(&d, "stocks", t.Ticker, "financial_data.yaml")
	isin := firstNonEmpty(d.ISIN, t.ID)
	country := strings.ToUpper(d.Identity.Country)
	if t.Item != nil && country == "" {
		country = strings.ToUpper(t.Item.Country)
	}
	us := strings.HasPrefix(strings.ToUpper(isin), "US") || country == "US"
	cfg := livetool.Config{Dir: e.ledgerDir(t.Ticker), CacheDir: cacheRoot(), Now: e.now(), Ticker: symbolOf(t), US: us}
	if us {
		cfg.Symbol = firstNonEmpty(d.Ticker, isin, symbolOf(t))
	} else {
		var hv HumanView
		if ok, _ := e.readYAML(&hv, "stocks", t.Ticker, "human_view.yaml"); ok {
			cfg.BrokerID = hv.BrokerID
		}
		if resolve := brokerIDResolvers[strings.ToLower(pack.Active().Broker.Key)]; cfg.BrokerID == "" && resolve != nil {
			id, err := resolve(cmd, symbolOf(t), t.Item)
			if err != nil {
				return nil, err
			}
			cfg.BrokerID = id
		}
	}
	s, err := livetool.NewSet(cfg)
	if err != nil {
		return nil, err
	}
	if s.News == nil {
		if us {
			return nil, fmt.Errorf("no news feed for %s: EULERPOOL_API_KEY is not set", t.Ticker)
		}
		return nil, fmt.Errorf("no news feed for %s: %w (a broker connector with a news feed is needed)", t.Ticker, livetool.ErrNoFeed)
	}
	return s.News, nil
}

// noBrokerSession says the broker's feed is out of reach: no connector, no session or an expired
// one (a connector wraps those in livetool.ErrNoFeed).
func noBrokerSession(err error) bool { return errors.Is(err, livetool.ErrNoFeed) }

// savedNewsShown is how many saved headlines the fallback prints.
const savedNewsShown = 25

// savedNews prints the headlines already in the name's news.yaml (from a connector's sync) when the
// live feed cannot be reached.
func (e *env) savedNews(arg string, cause error) error {
	ts, err := e.targets([]string{arg}, false, "")
	if err != nil {
		return err
	}
	t := ts[0]
	dir := e.ledgerDir(t.Ticker)
	l, err := livetool.ReadNewsLedger(dir)
	if err != nil {
		return err
	}
	items := slices.Clone(l.Items)
	slices.SortStableFunc(items, func(a, b livetool.NewsRecord) int { return strings.Compare(b.PublishedAt, a.PublishedAt) })
	newest := ""
	for _, it := range items {
		newest = max(newest, it.RetrievedAt)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The live broker news feed is out of reach (%v). Refresh the saved headlines with your broker connector's sync.\n\n", cause)
	if len(items) == 0 {
		fmt.Fprintf(&b, "%s has no saved headlines in %s: its news is DATA UNAVAILABLE until a sync saves some.\n", t.Ticker, filepath.Join(dir, livetool.NewsLedgerFile))
	} else {
		fmt.Fprintf(&b, "Saved headlines for %s in %s (%d, last retrieved %s; cite them with that date):\n", t.Ticker, filepath.Join(dir, livetool.NewsLedgerFile), len(items), newest)
		for _, it := range items[:min(savedNewsShown, len(items))] {
			fmt.Fprintf(&b, "- %s  %s  [%s %s] %s\n", day10s(it.PublishedAt), it.Headline, it.Vendor, it.ArticleID, it.Publisher)
		}
	}
	_, err = fmt.Fprint(e.out, b.String())
	return err
}

// BrokerIDResolver finds a non-US name's instrument id at the broker (a number or a UUID, as the
// broker writes it), for its news feed. A broker connector registers one with
// RegisterBrokerIDResolver; with none, a name needs the id in its human_view.yaml (broker_id) or
// its news comes from the saved ledger.
type BrokerIDResolver func(cmd *cobra.Command, symbol string, item *WatchItem) (string, error)

var brokerIDResolvers = map[string]BrokerIDResolver{}

// RegisterBrokerIDResolver installs the instrument-id lookup pb news uses while the broker pack
// named key is active.
func RegisterBrokerIDResolver(key string, f BrokerIDResolver) {
	brokerIDResolvers[strings.ToLower(strings.TrimSpace(key))] = f
}

// symbolOf is the ticker a name is looked up by: the watch item's, else the target's.
func symbolOf(t target) string {
	if t.Item != nil {
		return strings.ToUpper(t.Item.Ticker)
	}
	return t.Ticker
}
