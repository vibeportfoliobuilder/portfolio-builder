// retryscope_test.go - WHICH statuses get a retry, and for whom. retry_test.go
// beside it covers how long a retry waits.
package httpx

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestA404IsOneRequestUnlessTheCallerAsksForMore: for six sources a 404 is
// routine coverage, while one publisher's KID address flaps 404 between runs.
func TestA404IsOneRequestUnlessTheCallerAsksForMore(t *testing.T) {
	// A schedule of zero-length waits: its LENGTH is the number of retries, so
	// nothing here sits through the real eight seconds to count them.
	schedule := make([]time.Duration, len(DefaultBackoff()))
	const retried = 1 + 2 // the first attempt, plus the schedule's two retries

	for _, tc := range []struct {
		name    string
		status  int
		opts    []Option
		want    int
		because string
	}{
		{"404 by default", http.StatusNotFound, nil, 1,
			"a 404 is an answer to every caller that has not said otherwise"},
		{"404 asked for", http.StatusNotFound, []Option{RetryAlso(http.StatusNotFound)}, retried,
			"the caller that measured the flap must still get its retries"},
		{"503 by default", http.StatusServiceUnavailable, nil, retried,
			"a shared status is retried without anybody asking"},
		{"503 under an unrelated opt-in", http.StatusServiceUnavailable, []Option{RetryAlso(http.StatusNotFound)}, retried,
			"an opt-in adds to the shared list, it does not replace it"},
		{"403 asked for", http.StatusForbidden, []Option{RetryAlso(http.StatusForbidden)}, retried,
			"the option is a list of statuses, not a 404 flag"},
		{"403 by default", http.StatusForbidden, nil, 1,
			"a refusal is about the request, and repeating it is only rude"},
		{"400 while 404 is asked for", http.StatusBadRequest, []Option{RetryAlso(http.StatusNotFound)}, 1,
			"naming one status must not retry its neighbours"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := serveStatus(t, tc.status, nil)
			resp, attempts, err := Do(context.Background(), srv.Client(), NewPacer(0), schedule, getFrom(srv), tc.opts...)
			if err != nil {
				t.Fatalf("fetchDo: %v", err)
			}
			resp.Body.Close()
			if *hits != tc.want || attempts != tc.want {
				t.Errorf("http %d: %d requests (%d attempts reported), want %d - %s",
					tc.status, *hits, attempts, tc.want, tc.because)
			}
		})
	}
}

// TestRetryAlsoIsReachableFromOutsideThisPackage: the opt-in is useless unless
// the exported Do and RetryAlso carry the rule the unexported loop does.
func TestRetryAlsoIsReachableFromOutsideThisPackage(t *testing.T) {
	schedule := make([]time.Duration, len(DefaultBackoff()))
	srv, hits := serveStatus(t, http.StatusNotFound, nil)

	resp, _, err := Do(context.Background(), srv.Client(), NewPacer(0), schedule, getFrom(srv))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if *hits != 1 {
		t.Fatalf("Do made %d requests for a 404, want 1", *hits)
	}

	resp, _, err = Do(context.Background(), srv.Client(), NewPacer(0), schedule, getFrom(srv), RetryAlso(http.StatusNotFound))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if *hits != 1+len(schedule)+1 {
		t.Errorf("Do with RetryAlso(404) brought the total to %d requests, want %d", *hits, 1+len(schedule)+1)
	}
}
