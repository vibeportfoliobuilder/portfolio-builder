package lightyear

// explore.go - the explore page's fund screener: one page of twenty, and the walk
// over all of them. Identity comes from the instrument block, the count from the screener.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// PageSize is how many rows one page carries. The page has no size parameter -
// asking for a larger one is ignored - so it is the walk's stride, not a request.
const PageSize = 20

// maxPages bounds the walk at fifty times PageSize, so a total that never
// converges costs a bounded number of requests rather than an unbounded one.
const maxPages = 50

// Fund is one listing as the explore page states it. A fund listed on three
// venues is three of these, sharing an ISIN.
type Fund struct {
	Ticker   string
	Name     string
	ISIN     string
	Exchange string // the broker's own venue label: LSE, XETRA, AEX, PAR
	MIC      string
	Currency string // the LISTING currency, not the fund's base currency
	Class    string // Acc / Dist, empty when the page states neither

	// IssueType is ETF, ETC, ETN or MUTUAL_FUND, and AssetType the screener's own
	// grouping - "Equity ETFs", "Money Market Funds", "Unknown".
	IssueType string
	AssetType string

	Manager string
}

// Page is one page of the screener: its rows, and how many rows the screener
// holds in all, which is what bounds the walk.
type Page struct {
	Funds []Fund
	Total int
}

// screenerRow is what the fund table's columns show, joined to an instrument by id.
type screenerRow struct {
	InstrumentID string `json:"instrumentId"`
	AssetType    string `json:"assetType"`
	FundManager  string `json:"fundManager"`
}

// ExplorePath is the page this package reads, in English so the column labels
// and the asset-type names are the ones the decoder was written against.
func ExplorePath(page int) string {
	if page <= 1 {
		return "/en/funds/explore"
	}
	return "/en/funds/explore?page=" + strconv.Itoa(page)
}

// Page fetches and decodes one page of the screener, numbered from 1.
func (c *Client) Page(ctx context.Context, page int) (Page, error) {
	body, err := c.get(ctx, ExplorePath(page), cacheKey(page))
	if err != nil {
		return Page{}, err
	}
	p, err := DecodePage(body)
	if err != nil {
		return Page{}, fmt.Errorf("%w (page %d)", err, page)
	}
	return p, nil
}

// DecodePage reads one explore page. Exported so a fixture is decoded by the
// same function a download is.
func DecodePage(body []byte) (Page, error) {
	env, err := readEnvelope(body)
	if err != nil {
		return Page{}, err
	}
	var rows []screenerRow
	if err := json.Unmarshal(env.rows, &rows); err != nil {
		return Page{}, fmt.Errorf("lightyear: screener rows: %w", err)
	}

	// The screener's order is the table's order and so the walk's, which is what
	// makes two runs over an unchanged screener produce the same rows.
	out := make([]Fund, 0, len(rows))
	for _, r := range rows {
		in, ok := env.byID[r.InstrumentID]
		if !ok {
			continue
		}
		out = append(out, Fund{
			Ticker:    in.Symbol,
			Name:      in.Name,
			ISIN:      in.ISIN,
			Exchange:  in.Exchange,
			MIC:       in.MIC,
			Currency:  in.Currency,
			Class:     in.Summary.DistributionPolicy,
			IssueType: in.IssueType,
			AssetType: r.AssetType,
			Manager:   r.FundManager,
		})
	}
	if len(out) == 0 {
		return Page{}, errors.New("lightyear: the page carries a screener envelope but no row joined an instrument")
	}
	return Page{Funds: out, Total: env.total}, nil
}

// Funds walks every page of the screener. progress, if given, is called after
// each page with what is held so far and the count the screener declares.
func (c *Client) Funds(ctx context.Context, progress func(have, total int)) ([]Fund, error) {
	return walkPages(ctx, maxPages, func(ctx context.Context, n int) ([]Fund, int, error) {
		p, err := c.Page(ctx, n)
		return p.Funds, p.Total, err
	}, progress, "funds", "screener")
}

// walkPages reads pages from 1 until the rows reach the declared total, at most
// limit of them; noun names the rows and screener the walk in its errors.
func walkPages[T any](ctx context.Context, limit int, page func(context.Context, int) ([]T, int, error),
	progress func(have, total int), noun, screener string,
) ([]T, error) {
	var out []T
	for n := 1; n <= limit; n++ {
		rows, total, err := page(ctx, n)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if progress != nil {
			progress(len(out), total)
		}
		if len(out) >= total {
			return out, nil
		}
		// A page shorter than the stride is the last one; without this a screener
		// that shrinks mid-walk would ask for pages that no longer exist.
		if len(rows) < PageSize {
			return out, fmt.Errorf("lightyear: the screener declares %d %s but ran out after %d", total, noun, len(out))
		}
	}
	return nil, fmt.Errorf("lightyear: the %s did not end within %d pages", screener, limit)
}
