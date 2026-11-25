package lightyear

// stocks.go - the explore page's OTHER screener: companies. The page shape is the
// fund one's, and the row carries the figures a screen would otherwise filter on.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// maxStockPages bounds the company walk. The company screener is an order of
// magnitude longer than the fund one, so it needs its own bound and not maxPages.
const maxStockPages = 500

// Stock is one company listing as the explore page states it. The figures are the
// screener's own columns, and a nil one is a column the page leaves null.
type Stock struct {
	Ticker string
	// InstrumentID is Lightyear's instrument uuid (public), the key a Lightyear connector reads a name by.
	InstrumentID string
	Name         string
	ISIN         string
	// MIC is the listing venue and the only venue key that is reliable: for the
	// Baltic lines the instrument's own Exchange echoes the MIC rather than a code.
	MIC      string
	Exchange string
	Currency string
	// IssueType is COMMON_STOCK, ADR or PREFERRED_STOCK, which is what separates a
	// company from a depositary receipt over one.
	IssueType string

	// Country is the COMPANY's, not the venue's: a Chinese issuer lists on NASDAQ
	// and states China here.
	Country  string
	Sector   string
	Industry string // measured always empty; see ReturnOnEquity

	// Price is in Currency; MarketCap likewise, and MarketCapUSD is the same figure
	// converted, which is the only one two venues can be compared on.
	Price        *float64
	ChangePct    *float64
	MarketCap    *float64
	MarketCapUSD *float64

	// Yield and PayoutRatio are PERCENTAGES as the page prints them, not fractions.
	PE          *float64
	Yield       *float64
	PayoutRatio *float64
	Beta        *float64

	// ReturnOnEquity, ReturnOnAssets and Industry are read because the page states
	// the keys, but it filled NONE of them on any of its 5,943 rows on 2026-09-17.
	ReturnOnEquity *float64
	ReturnOnAssets *float64

	// The growth columns are annualised percentages over the stated window.
	RevGrowth3y  *float64
	EPSGrowth3y  *float64
	DivGrowth3y  *float64
	RevGrowth10y *float64
	EPSGrowth10y *float64
	DivGrowth10y *float64
}

// StockPage is one page of the company screener: its rows, and the count the
// screener declares, which is what bounds the walk.
type StockPage struct {
	Stocks []Stock
	Total  int
}

// stockRow is what the company table's columns show. Every figure is a pointer
// because the page states null for one it does not have, which is not zero.
type stockRow struct {
	InstrumentID string `json:"instrumentId"`
	Country      string `json:"country"`
	Sector       string `json:"sector"`
	Industry     string `json:"industry"`

	Price        *float64 `json:"price"`
	ChangePct    *float64 `json:"changePercentOneDay"`
	MarketCap    *float64 `json:"marketCap"`
	MarketCapUSD *float64 `json:"marketCapInUsd"`

	PE          *float64 `json:"peRatio"`
	Yield       *float64 `json:"dividendYield"`
	PayoutRatio *float64 `json:"payoutRatio"`
	Beta        *float64 `json:"beta"`

	ReturnOnEquity *float64 `json:"returnOnEquity"`
	ReturnOnAssets *float64 `json:"returnOnAssets"`

	RevGrowth3y  *float64 `json:"threeYearRevenueGrowth"`
	EPSGrowth3y  *float64 `json:"threeYearEpsGrowth"`
	DivGrowth3y  *float64 `json:"threeYearDividendGrowth"`
	RevGrowth10y *float64 `json:"tenYearRevenueGrowth"`
	EPSGrowth10y *float64 `json:"tenYearEpsGrowth"`
	DivGrowth10y *float64 `json:"tenYearDividendGrowth"`
}

// StockPath is the company explore page, in English for the same reason
// ExplorePath is: the column labels are the ones the decoder was written against.
func StockPath(page int) string {
	if page <= 1 {
		return "/en/stocks/explore"
	}
	return "/en/stocks/explore?page=" + strconv.Itoa(page)
}

// stockCacheKey names one page of the company walk, kept apart from the fund
// walk's keys because the two screeners page independently.
func stockCacheKey(page int) string {
	return "lightyear/stocks/page-" + strconv.Itoa(page) + ".html"
}

// StockPageAt fetches and decodes one page of the company screener, numbered from 1.
func (c *Client) StockPageAt(ctx context.Context, page int) (StockPage, error) {
	body, err := c.get(ctx, StockPath(page), stockCacheKey(page))
	if err != nil {
		return StockPage{}, err
	}
	p, err := DecodeStockPage(body)
	if err != nil {
		return StockPage{}, fmt.Errorf("%w (page %d)", err, page)
	}
	return p, nil
}

// DecodeStockPage reads one company explore page. Exported so a fixture is
// decoded by the same function a download is.
func DecodeStockPage(body []byte) (StockPage, error) {
	env, err := readEnvelope(body)
	if err != nil {
		return StockPage{}, err
	}
	var rows []stockRow
	if err := json.Unmarshal(env.rows, &rows); err != nil {
		return StockPage{}, fmt.Errorf("lightyear: screener rows: %w", err)
	}

	// The screener's order is the table's order and so the walk's, which is what
	// makes two runs over an unchanged screener produce the same rows.
	out := make([]Stock, 0, len(rows))
	for _, r := range rows {
		in, ok := env.byID[r.InstrumentID]
		if !ok {
			continue
		}
		// The instrument's summary states a sector too; the row's is the one the
		// table prints, so it wins and the summary only fills a blank.
		sector := r.Sector
		if sector == "" {
			sector = in.Summary.Sector
		}
		out = append(out, Stock{
			InstrumentID: r.InstrumentID,
			Ticker:       in.Symbol,
			Name:         in.Name,
			ISIN:         in.ISIN,
			MIC:          in.MIC,
			Exchange:     in.Exchange,
			Currency:     in.Currency,
			IssueType:    in.IssueType,

			Country:  r.Country,
			Sector:   sector,
			Industry: r.Industry,

			Price:        r.Price,
			ChangePct:    r.ChangePct,
			MarketCap:    r.MarketCap,
			MarketCapUSD: r.MarketCapUSD,

			PE:          r.PE,
			Yield:       r.Yield,
			PayoutRatio: r.PayoutRatio,
			Beta:        r.Beta,

			ReturnOnEquity: r.ReturnOnEquity,
			ReturnOnAssets: r.ReturnOnAssets,

			RevGrowth3y:  r.RevGrowth3y,
			EPSGrowth3y:  r.EPSGrowth3y,
			DivGrowth3y:  r.DivGrowth3y,
			RevGrowth10y: r.RevGrowth10y,
			EPSGrowth10y: r.EPSGrowth10y,
			DivGrowth10y: r.DivGrowth10y,
		})
	}
	if len(out) == 0 {
		return StockPage{}, errors.New("lightyear: the page carries a screener envelope but no row joined an instrument")
	}
	return StockPage{Stocks: out, Total: env.total}, nil
}

// Stocks walks every page of the company screener. progress, if given, is called
// after each page with what is held so far and the count the screener declares.
func (c *Client) Stocks(ctx context.Context, progress func(have, total int)) ([]Stock, error) {
	return walkPages(ctx, maxStockPages, func(ctx context.Context, n int) ([]Stock, int, error) {
		p, err := c.StockPageAt(ctx, n)
		return p.Stocks, p.Total, err
	}, progress, "companies", "company screener")
}
