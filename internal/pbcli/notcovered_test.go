package pbcli

// notcovered_test.go - a screen stock Eulerpool does not carry (XOM) is
// flagged on the Screen page instead of silently dropped.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

func TestEnrichFlagsAStockEulerpoolDoesNotCover(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Security not found"}`))
	}))
	defer srv.Close()
	cl := eulerpool.NewClientWithKey("k")
	cl.Base, cl.Pacer, cl.Cache = srv.URL+"/api/1/", nil, nil
	doc := &ScreenDoc{Stocks: []ScreenCandidate{{Ticker: "XOM", ISIN: "US0000000105"}}}
	errs := enrichFromEulerpool(context.Background(), cl, doc)
	if len(errs) != 1 || len(doc.Stocks[0].Flags) != 1 || doc.Stocks[0].Flags[0] != NotCoveredFlag {
		t.Fatalf("errs %v flags %v", errs, doc.Stocks[0].Flags)
	}
}
