package livetool

// newsep_test.go - the Eulerpool half of the news pair: rows carry an address, and Read hands it back.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/httpx"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

const twoEulerpoolItems = `[
 {"headline":"Arista beats","summary":"s","source":"Reuters","url":"https://example.com/a","datetime":"1790250171"},
 {"headline":"Arista guides up","summary":"s","source":"Benzinga","url":"https://example.com/b","datetime":"1790250000"}]`

func stubEulerpool(t *testing.T, status int) *eulerpool.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twoEulerpoolItems))
	}))
	t.Cleanup(srv.Close)
	c := eulerpool.NewClientWithKey("test-key")
	c.HTTP, c.Base, c.Pacer, c.Backoff = srv.Client(), srv.URL+"/", httpx.NewPacer(0), []time.Duration{}
	return c
}

func TestAnEulerpoolWalkRecordsAddressedRows(t *testing.T) {
	dir := t.TempDir()
	n := NewNewsroom(nil, "", "NETW", dir, day("2026-11-06")).WithEulerpool(stubEulerpool(t, http.StatusOK))
	out, err := n.Search(t.Context(), NewsArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2 items for NETW") {
		t.Fatalf("listing: %s", out)
	}
	l, err := ReadNewsLedger(dir)
	if err != nil || len(l.Items) != 2 {
		t.Fatalf("ledger: %+v %v", l, err)
	}
	for _, r := range l.Items {
		if r.Vendor != vendorEulerpool || r.URL == "" || !strings.HasPrefix(r.ArticleID, "ep-") {
			t.Errorf("row %+v", r)
		}
	}
	got, err := n.Read(t.Context(), ReadArticleArgs{ArticleID: l.Items[0].ArticleID})
	if err != nil || !strings.Contains(got, "https://example.com/") {
		t.Fatalf("read: %q %v", got, err)
	}
}

func TestAFailedEulerpoolFeedAloneIsAnError(t *testing.T) {
	n := NewNewsroom(nil, "", "NETW", t.TempDir(), day("2026-11-06")).WithEulerpool(stubEulerpool(t, http.StatusInternalServerError))
	if _, err := n.Search(t.Context(), NewsArgs{}); err == nil {
		t.Fatal("a walk whose only feed failed returned no error")
	}
}
