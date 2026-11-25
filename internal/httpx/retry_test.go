// retry_test.go - the Retry-After rule: a header, a fallback and a cap. The same
// loop through a document download is exercised in internal/etf/pipeline.
package httpx

import (
	"net/http"
	"testing"
	"time"
)

// The server's own Retry-After wins over our backoff, within reason: what we owe
// a server asking for an hour is to stop, not to come back sooner.
func TestRetryAfterIsHonouredWithinReason(t *testing.T) {
	const fallback = 2 * time.Second
	for _, tc := range []struct {
		header  string
		want    time.Duration
		canWait bool
	}{
		{"", fallback, true},
		{"5", 5 * time.Second, true},
		{"30", 30 * time.Second, true},
		{"3600", 0, false}, // longer than a run can wait: stop, do not shorten it
		{"0", 0, true},     // RFC 9110: retry immediately
		{"-1", fallback, true},
		{"soon", fallback, true},
	} {
		resp := &http.Response{Header: http.Header{}}
		if tc.header != "" {
			resp.Header.Set("Retry-After", tc.header)
		}
		got, canWait := retryAfter(resp, fallback)
		if got != tc.want || canWait != tc.canWait {
			t.Errorf("Retry-After %q -> %v, %v, want %v, %v", tc.header, got, canWait, tc.want, tc.canWait)
		}
	}

	// The date form, which some servers use instead of a count of seconds.
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", time.Now().Add(4*time.Second).UTC().Format(http.TimeFormat))
	if got, canWait := retryAfter(resp, fallback); !canWait || got < 2*time.Second || got > 5*time.Second {
		t.Errorf("a date four seconds out gave %v, %v", got, canWait)
	}
	resp.Header.Set("Retry-After", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	if got, canWait := retryAfter(resp, fallback); canWait {
		t.Errorf("a date an hour out gave %v, %v - want the retry stopped", got, canWait)
	}
	// A date already gone is the same statement as `Retry-After: 0`.
	resp.Header.Set("Retry-After", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat))
	if got, canWait := retryAfter(resp, fallback); !canWait || got != 0 {
		t.Errorf("a date an hour past gave %v, %v - want no wait at all", got, canWait)
	}
}
