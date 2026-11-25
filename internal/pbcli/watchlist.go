package pbcli

// watchlist.go - data/watchlist.yaml and how a command's arguments become names.

import (
	"fmt"
	"strings"
)

// Watchlist is data/watchlist.yaml, written by pb watchlist from-picks (gate 1) and pb watchlist add.
type Watchlist struct {
	AsOf   string      `yaml:"as_of" json:"as_of"`
	Source string      `yaml:"source" json:"source"`
	Items  []WatchItem `yaml:"items" json:"items"`
}

// WatchItem is one watched instrument. Kind is stock or etf.
type WatchItem struct {
	Ticker       string `yaml:"ticker" json:"ticker"`
	Name         string `yaml:"name,omitempty" json:"name,omitempty"`
	Kind         string `yaml:"kind" json:"kind"`
	ISIN         string `yaml:"isin,omitempty" json:"isin,omitempty"`
	IssueType    string `yaml:"issue_type,omitempty" json:"issue_type,omitempty"`
	Exchange     string `yaml:"exchange,omitempty" json:"exchange,omitempty"`
	MIC          string `yaml:"mic,omitempty" json:"mic,omitempty"`
	Currency     string `yaml:"currency,omitempty" json:"currency,omitempty"`
	Country      string `yaml:"country,omitempty" json:"country,omitempty"`
	Sector       string `yaml:"sector,omitempty" json:"sector,omitempty"`
	InstrumentID string `yaml:"instrument_id,omitempty" json:"instrument_id,omitempty"`
}

// The two kinds a watch item can be.
const (
	kindStock = "stock"
	kindETF   = "etf"
)

// kindOf maps a broker's issue type to stock or etf.
func kindOf(issueType string) string {
	switch strings.ToUpper(issueType) {
	case "ETF", "ETC", "ETN", "MUTUAL_FUND", "FUND":
		return kindETF
	}
	return kindStock
}

// target is one name a command works on.
type target struct {
	Ticker string // folder key under data/stocks or data/etfs
	ID     string // ISIN where known, else the ticker as given
	Item   *WatchItem
}

// readWatchlist loads data/watchlist.yaml; an absent file is an empty list.
func (e *env) readWatchlist() (Watchlist, error) {
	var w Watchlist
	_, err := e.readYAML(&w, "watchlist.yaml")
	return w, err
}

// targets turns args (TICKER, ISIN or TICKER=ISIN) or --watchlist into names of one kind.
// kind "" accepts both kinds.
func (e *env) targets(args []string, watchlist bool, kind string) ([]target, error) {
	w, err := e.readWatchlist()
	if err != nil {
		return nil, err
	}
	if watchlist {
		if len(w.Items) == 0 {
			return nil, fmt.Errorf("%s has no items: run pb ly import first", e.path("watchlist.yaml"))
		}
		var out []target
		for i := range w.Items {
			it := &w.Items[i]
			if kind != "" && it.Kind != kind {
				continue
			}
			out = append(out, target{Ticker: dirName(it.Ticker), ID: firstNonEmpty(it.ISIN, it.Ticker), Item: it})
		}
		return out, nil
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("name at least one ticker or ISIN, or pass --watchlist")
	}
	var out []target
	for _, a := range args {
		t, id, _ := strings.Cut(a, "=")
		t = strings.TrimSpace(t)
		var item *WatchItem
		for i := range w.Items {
			if strings.EqualFold(w.Items[i].Ticker, t) || (w.Items[i].ISIN != "" && strings.EqualFold(w.Items[i].ISIN, t)) {
				item = &w.Items[i]
				break
			}
		}
		switch {
		case id != "":
		case item != nil:
			id = firstNonEmpty(item.ISIN, item.Ticker)
			t = item.Ticker
		default:
			id = t
		}
		out = append(out, target{Ticker: dirName(t), ID: strings.TrimSpace(id), Item: item})
	}
	return out, nil
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}
