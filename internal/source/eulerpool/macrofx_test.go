package eulerpool

// macrofx_test.go - the FX block of pb ep macro on made-up ECB fixings: every pair is quoted in the
// home currency, crossed through the EUR the ECB fixes everything against.

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fakeKey = "made-up-test-key"

// fakeECB serves made-up EUR fixings for two dates; every other route is a 404 (a data gap).
func fakeECB(t *testing.T) *Client {
	t.Helper()
	rates := map[string]string{
		"USD": `{"data":[{"date":"2026-10-01","rate":1.20},{"date":"2026-11-02","rate":1.25}]}`,
		"GBP": `{"data":[{"date":"2026-10-01","rate":0.80},{"date":"2026-11-02","rate":0.85}]}`,
		"SEK": `{"data":[{"date":"2026-10-01","rate":11.0},{"date":"2026-11-02","rate":11.5}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := strings.TrimPrefix(r.URL.Path, "/api/1/")
		if cur, ok := strings.CutPrefix(route, "ecb/exchange-rates/"); ok && rates[cur] != "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rates[cur]))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not here"}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClientWithKey(fakeKey)
	c.Base = srv.URL + "/api/1/"
	c.Now = func() time.Time { return time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC) }
	c.Backoff = []time.Duration{}
	c.Pacer = nil
	return c
}

func pairs(t *testing.T, m *Macro) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	for _, r := range m.FX {
		out[r.Pair] = r.Rate
	}
	return out
}

func TestMacroFXInAEuroHome(t *testing.T) {
	m, err := fakeECB(t).Macro(context.Background(), nil, MacroOptions{Home: "EUR", BaseAreas: []string{"US", "GB"}, Currencies: []string{"GBP"}})
	if err != nil {
		t.Fatal(err)
	}
	fx := pairs(t, m)
	if math.Abs(fx["USD/EUR"]-0.8) > 1e-4 || math.Abs(fx["GBP/EUR"]-1/0.85) > 1e-4 {
		t.Errorf("fx %v, want USD/EUR 0.8 and GBP/EUR 1.1765", fx)
	}
	if _, ok := fx["EUR/EUR"]; ok {
		t.Error("the home currency was quoted against itself")
	}
	if got := strings.Join(m.Countries, ","); got != "US,GB" {
		t.Errorf("countries %s, want the base areas", got)
	}
}

func TestMacroFXInAnotherHome(t *testing.T) {
	m, err := fakeECB(t).Macro(context.Background(), []string{"de"}, MacroOptions{Home: "SEK", BaseAreas: []string{"US"}})
	if err != nil {
		t.Fatal(err)
	}
	fx := pairs(t, m)
	// SEK per USD = (SEK per EUR) / (USD per EUR) = 11.5 / 1.25; SEK per EUR = 11.5.
	if math.Abs(fx["USD/SEK"]-9.2) > 1e-4 || math.Abs(fx["EUR/SEK"]-11.5) > 1e-4 {
		t.Errorf("fx %v, want USD/SEK 9.2 and EUR/SEK 11.5", fx)
	}
	if got := strings.Join(m.Countries, ","); got != "US,DE" {
		t.Errorf("countries %s, want the base area and the watchlist's", got)
	}
}
