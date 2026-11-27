package pbcli

// newslist.go - pb news list: the saved headlines of news.yaml as one line each, newest first,
// optionally with cross-vendor duplicates folded and known noise dropped (always counted).

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/agent/livetool"
)

// NewsLine is one listed headline. Also names the other publishers (and their ids) that carried
// the same headline when --dedupe folded them into this line.
type NewsLine struct {
	ID        string   `yaml:"id" json:"id"`
	Date      string   `yaml:"date" json:"date"`
	Publisher string   `yaml:"publisher" json:"publisher"`
	Headline  string   `yaml:"headline" json:"headline"`
	Also      []string `yaml:"also,omitempty" json:"also,omitempty"`
}

// NewsListing is one name's listing: what was in the file, what is shown and what was left out.
type NewsListing struct {
	Ticker  string         `yaml:"ticker" json:"ticker"`
	File    string         `yaml:"file" json:"file"`
	Total   int            `yaml:"total" json:"total"`
	Shown   int            `yaml:"shown" json:"shown"`
	Dropped map[string]int `yaml:"dropped,omitempty" json:"dropped,omitempty"`
	Items   []NewsLine     `yaml:"items" json:"items"`
}

// Reasons a row is left out of a listing; each is counted, never silent.
const (
	dropOlder     = "older than --since"
	dropDuplicate = "duplicate headline (folded into the first: see also)"
	dropLimit     = "over --limit"
	dropNoID      = "no article id"
)

// newsNoise is the documented noise list --dedupe drops: rows that carry no news about the
// business. A rule names a publisher, a headline pattern, or both; the reason is what is counted.
var newsNoise = []struct {
	reason    string
	publisher string // exact, case-insensitive; "" = any
	headline  *regexp.Regexp
}{
	{"noise: IB Market Signals price/volume signal", "IB Market Signals", nil},
	{"noise: exchange order-imbalance notice", "", regexp.MustCompile(`(?i)\border imbalance\b`)},
	// Listicles: "3 Dividend Kings to Buy", "These 5 Consumer Stocks ...", "How many shares of X you'd need".
	{"noise: listicle", "", regexp.MustCompile(`(?i)^(these |the |my )?\d+ [^.:]*\b(stocks?|kings?|aristocrats?|raisers?|picks?|etfs?|reits?|names|companies)\b`)},
	{"noise: listicle", "", regexp.MustCompile(`(?i)\bhow many shares of\b|\bif you (had )?invest(ed)? \$|\bhere'?s what \$[\d,]+|^prediction:|\bthese \d+ [^.:]*\bstocks\b`)},
	// A vendor's generic sector page or weekly roundup; a wire's research roundup (rating changes) stays.
	{"noise: vendor sector or industry report", "", regexp.MustCompile(`(?i)^(sector|industry) report for\b|^iss eva research roundup\b`)},
}

// noiseReason is the rule a row matches, or "".
func noiseReason(r livetool.NewsRecord) string {
	for _, n := range newsNoise {
		if n.publisher != "" && !strings.EqualFold(strings.TrimSpace(r.Publisher), n.publisher) {
			continue
		}
		if n.headline != nil && !n.headline.MatchString(r.Headline) {
			continue
		}
		return n.reason
	}
	return ""
}

var (
	// Wire tags around the same story: "BRIEF-", "UPDATE 2-", "(CNW) " in front; " -- WSJ",
	// " -- Update", " - Draft" behind.
	wirePrefix = regexp.MustCompile(`(?i)^\s*(brief|update( \d+)?|exclusive|corrected)\s*-\s*|^\s*\([a-z .]{2,20}\)\s*`)
	wireSuffix = regexp.MustCompile(`(?i)\s+--\s*[^-]{1,24}$|\s+-\s+draft$`)
	nonAlnum   = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// headlineKey is a headline normalized for --dedupe: wire tags off, lower case, letters and digits only.
func headlineKey(h string) string {
	h = wireSuffix.ReplaceAllString(wirePrefix.ReplaceAllString(h, ""), "")
	return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(h), " "))
}

// newsOpts are pb news list's filters.
type newsOpts struct {
	Since  string
	Limit  int
	Dedupe bool
}

// listNews turns a ledger's rows into the listing, newest first. With Dedupe the earliest row of
// a headline keeps its line and the rest are named under also; noise rules drop rows. Every row
// left out is counted under its reason.
func listNews(rows []livetool.NewsRecord, o newsOpts) (items []NewsLine, dropped map[string]int) {
	dropped = map[string]int{}
	rows = slices.Clone(rows)
	// Oldest first, so the first sighting of a headline is the one kept.
	slices.SortStableFunc(rows, func(a, b livetool.NewsRecord) int { return strings.Compare(a.PublishedAt, b.PublishedAt) })
	byKey := map[string]int{}
	for _, r := range rows {
		switch {
		case strings.TrimSpace(r.ArticleID) == "":
			dropped[dropNoID]++
			continue
		case o.Since != "" && day10s(r.PublishedAt) < o.Since:
			dropped[dropOlder]++
			continue
		}
		if o.Dedupe {
			if why := noiseReason(r); why != "" {
				dropped[why]++
				continue
			}
			k := headlineKey(r.Headline)
			if i, seen := byKey[k]; seen && k != "" {
				dropped[dropDuplicate]++
				if a := r.Publisher + " " + r.ArticleID; !strings.EqualFold(r.Publisher, items[i].Publisher) && !slices.ContainsFunc(items[i].Also, func(s string) bool { return strings.HasPrefix(s, r.Publisher+" ") }) {
					items[i].Also = append(items[i].Also, a)
				}
				continue
			}
			byKey[k] = len(items)
		}
		items = append(items, NewsLine{ID: r.ArticleID, Date: day10s(r.PublishedAt), Publisher: strings.TrimSpace(r.Publisher),
			Headline: strings.Join(strings.Fields(r.Headline), " ")})
	}
	slices.Reverse(items)
	if o.Limit > 0 && len(items) > o.Limit {
		dropped[dropLimit] += len(items) - o.Limit
		items = items[:o.Limit]
	}
	if len(dropped) == 0 {
		dropped = nil
	}
	return items, dropped
}

func newsListCmd(e *env) *cobra.Command {
	var watch, dedupe bool
	var tickers []string
	var since string
	var limit int
	c := &cobra.Command{
		Use:   "list <TICKER ...> | --watchlist [--tickers A,B] [--since YYYY-MM-DD] [--limit N] [--dedupe]",
		Short: "List the saved headlines of news.yaml, one line each (id date publisher: headline), newest first",
		Long: `Reads data/stocks/<T>/news.yaml (data/etfs/<T>/ for a fund) without any network call and prints
one line per item, newest first: "<article_id> <date> <publisher>: <headline>". Cite an item by its
id and publisher; pb news read <T> <id> opens it.

--dedupe folds the same headline carried by several publishers or vendors into the earliest item
(wire tags such as "BRIEF-", "(CNW)", " -- WSJ" and case and punctuation are ignored) and lists the
others after "also:" with their ids; it also drops noise that says nothing about the business:
IB Market Signals price and volume signals, order-imbalance notices, listicles ("3 Dividend Kings
to Buy", "How many shares of X you'd need", "If you invested $..."), and vendor sector or industry
reports. Every row left out is counted under its reason; run without --dedupe to see them all.
--tickers narrows the names (with --watchlist: the watchlist names among them).
Examples:
  pb news list KO --dedupe --since 2026-10-01 --md
  pb news list --watchlist --tickers KO,O --dedupe --limit 30 --md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var want []string
			for _, t := range tickers {
				for _, s := range strings.Split(t, ",") {
					if s = strings.TrimSpace(s); s != "" {
						want = append(want, s)
					}
				}
			}
			if !watch {
				args = append(args, want...)
			}
			ts, err := e.targets(args, watch, "")
			if err != nil {
				return err
			}
			if watch && len(want) > 0 {
				ts = slices.DeleteFunc(ts, func(t target) bool {
					return !slices.ContainsFunc(want, func(w string) bool {
						return strings.EqualFold(w, symbolOf(t)) || strings.EqualFold(dirName(w), t.Ticker)
					})
				})
			}
			var out []NewsListing
			for _, t := range ts {
				dir := e.ledgerDir(t.Ticker)
				l, err := livetool.ReadNewsLedger(dir)
				if err != nil {
					return err
				}
				items, dropped := listNews(l.Items, newsOpts{Since: since, Limit: limit, Dedupe: dedupe})
				out = append(out, NewsListing{Ticker: t.Ticker, File: filepath.Join(dir, livetool.NewsLedgerFile), Total: len(l.Items),
					Shown: len(items), Dropped: dropped, Items: items})
			}
			if !e.md {
				return e.printJSON(map[string]any{"names": out})
			}
			_, err = fmt.Fprint(e.out, newsListMarkdown(out))
			return err
		},
	}
	c.Flags().BoolVar(&watch, "watchlist", false, "every name in data/watchlist.yaml")
	c.Flags().StringSliceVar(&tickers, "tickers", nil, "these names (comma-separated); with --watchlist, only these of it")
	c.Flags().StringVar(&since, "since", "", "only items published on or after this date (YYYY-MM-DD)")
	c.Flags().IntVar(&limit, "limit", 0, "at most this many lines per name (0: all)")
	c.Flags().BoolVar(&dedupe, "dedupe", false, "fold duplicate headlines across publishers and drop the documented noise")
	return c
}

// newsListMarkdown is the --md form: a heading per name with the counts, then one line per item.
func newsListMarkdown(ls []NewsListing) string {
	var b strings.Builder
	for _, l := range ls {
		fmt.Fprintf(&b, "## %s: %d of %d items in %s\n", l.Ticker, l.Shown, l.Total, l.File)
		if l.Total == 0 {
			b.WriteString("No saved headlines: the news is DATA UNAVAILABLE until a sync or pb news search saves some.\n")
		}
		if len(l.Dropped) > 0 {
			keys := make([]string, 0, len(l.Dropped))
			for k := range l.Dropped {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%d %s", l.Dropped[k], k))
			}
			fmt.Fprintf(&b, "Left out: %s.\n", strings.Join(parts, "; "))
		}
		for _, it := range l.Items {
			fmt.Fprintf(&b, "%s %s %s: %s", it.ID, it.Date, it.Publisher, it.Headline)
			if len(it.Also) > 0 {
				fmt.Fprintf(&b, " (also: %s)", strings.Join(it.Also, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}
