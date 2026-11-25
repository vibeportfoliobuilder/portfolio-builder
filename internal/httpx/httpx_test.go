package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

// The pacer's job is to be per-host and the test is when a call returns, so these
// are written to fail on the behaviour rather than on the jitter.

// TestHostPacerDoesNotBlockOtherHosts is the regression test for sleeping while
// holding the lock: a host mid-wait must not delay a request to another.
func TestHostPacerDoesNotBlockOtherHosts(t *testing.T) {
	const min = 200 * time.Millisecond
	p := NewPacer(min)
	ctx := context.Background()

	// Prime the slow host, so the next wait on it genuinely has to sleep.
	p.Wait(ctx, "slow.example")

	slow := make(chan struct{})
	go func() {
		defer close(slow)
		p.Wait(ctx, "slow.example")
	}()

	// Let the other goroutine get as far as its wait, which is where the old
	// code took the mutex and kept it for the whole interval.
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	p.Wait(ctx, "fast.example")
	if waited := time.Since(start); waited > min/2 {
		t.Fatalf("a request to an idle host waited %v behind another host's interval of %v", waited, min)
	}
	<-slow
}

// TestHostPacerSerialisesTheSameHost is the other half: three workers arriving
// at one host together have to leave it min apart.
func TestHostPacerSerialisesTheSameHost(t *testing.T) {
	const min = 100 * time.Millisecond
	p := NewPacer(min)
	ctx := context.Background()

	var mu sync.Mutex
	var at []time.Duration

	start := time.Now()
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Wait(ctx, "one.example")
			mu.Lock()
			at = append(at, time.Since(start))
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.Sort(at)

	// The three turns are at 0, min and 2*min. Measuring the span tolerates a
	// goroutine waking late, which can never make the run finish early.
	if span := at[len(at)-1] - at[0]; span < 2*min {
		t.Fatalf("three requests to one host spanned %v, want at least %v - they were not paced", span, 2*min)
	}
	if at[0] > min/2 {
		t.Fatalf("the first request to an idle host waited %v, want no wait at all", at[0])
	}
}

// TestHostPacerWithoutADelayDoesNotWait covers the setting most of this
// package's tests run with, and the -host-delay=0 a run can be given.
func TestHostPacerWithoutADelayDoesNotWait(t *testing.T) {
	p := NewPacer(0)
	ctx := context.Background()

	start := time.Now()
	for range 50 {
		if !p.Wait(ctx, "one.example") {
			t.Fatal("wait reported a cancelled run on a live context")
		}
	}
	if waited := time.Since(start); waited > 100*time.Millisecond {
		t.Fatalf("fifty unpaced requests took %v", waited)
	}
}

// TestHostPacerStopsWaitingWhenTheRunIsCancelled: a queue behind one issuer can
// be tens of seconds long, and Ctrl-C should not sit through it.
func TestHostPacerStopsWaitingWhenTheRunIsCancelled(t *testing.T) {
	p := NewPacer(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Wait(ctx, "one.example") // takes the first turn; the next is an hour off

	done := make(chan bool, 1)
	go func() { done <- p.Wait(ctx, "one.example") }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("wait reported success after the run was cancelled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not return when the run was cancelled")
	}
}

// the retry loop.

// serveStatus answers every request with one status, counting the requests.
func serveStatus(t *testing.T, code int, header map[string]string) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func getFrom(srv *httptest.Server) func() (*http.Request, error) {
	return func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, srv.URL, nil)
	}
}

// TestFetchRetriesOnTheCallersOwnSchedule: the schedule is a parameter, and its
// LENGTH is the number of retries.
func TestFetchRetriesOnTheCallersOwnSchedule(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backoff []time.Duration
		want    int
		// header is how the nil case is asked about without sitting through eight
		// seconds: the server says "retry immediately".
		header map[string]string
	}{
		{name: "none at all", backoff: []time.Duration{}, want: 1},
		{name: "one", backoff: []time.Duration{0}, want: 2},
		{name: "the default's length", backoff: make([]time.Duration, len(DefaultBackoff())), want: 3},
		{name: "nil takes the default", backoff: nil, want: 1 + len(DefaultBackoff()),
			header: map[string]string{"Retry-After": "0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := serveStatus(t, http.StatusServiceUnavailable, tc.header)
			resp, attempts, err := Do(context.Background(), srv.Client(), NewPacer(0), tc.backoff, getFrom(srv))
			if err != nil {
				t.Fatalf("fetchDo: %v", err)
			}
			resp.Body.Close()
			if *hits != tc.want || attempts != tc.want {
				t.Errorf("%d requests (%d attempts reported), want %d", *hits, attempts, tc.want)
			}
		})
	}
}

// TestFetchStopsWhenTheServerAsksForLongerThanTheRunCanWait: it used to fall
// through to our own backoff, so an origin asking for an hour got three requests.
func TestFetchStopsWhenTheServerAsksForLongerThanTheRunCanWait(t *testing.T) {
	srv, hits := serveStatus(t, http.StatusTooManyRequests, map[string]string{"Retry-After": "3600"})

	start := time.Now()
	resp, attempts, err := Do(context.Background(), srv.Client(), NewPacer(0),
		[]time.Duration{time.Minute, time.Minute}, getFrom(srv))
	if err != nil {
		t.Fatalf("fetchDo: %v", err)
	}
	defer resp.Body.Close()

	if *hits != 1 || attempts != 1 {
		t.Errorf("%d requests (%d attempts reported) against an origin that asked for an hour, want 1", *hits, attempts)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("took %v, so something waited: the point is that the run stops, not that it waits differently", took)
	}
	// The refusal comes back rather than becoming an error, so the caller can
	// record what the origin actually said.
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status %d, want the origin's own 429 handed back", resp.StatusCode)
	}
}

// TestFetchTreatsRetryAfterZeroAsRetryImmediately: RFC 9110 defines 0, which
// used to be rejected in favour of our own two seconds.
func TestFetchTreatsRetryAfterZeroAsRetryImmediately(t *testing.T) {
	srv, hits := serveStatus(t, http.StatusServiceUnavailable, map[string]string{"Retry-After": "0"})

	start := time.Now()
	resp, _, err := Do(context.Background(), srv.Client(), NewPacer(0),
		[]time.Duration{time.Minute, time.Minute}, getFrom(srv))
	if err != nil {
		t.Fatalf("fetchDo: %v", err)
	}
	resp.Body.Close()
	if *hits != 3 {
		t.Errorf("%d requests, want 3: a zero is a wait of no length, not an absent header", *hits)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("took %v, so the minute-long fallback was used where the server said zero", took)
	}
}

// TestACancelledRunDoesNotGetOneMoreAttempt: with a zero-length schedule both
// channels are ready and Go picks at random, so this asks many times.
func TestACancelledRunDoesNotGetOneMoreAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for range 200 {
		if SleepCtx(ctx, 0) {
			t.Fatal("a zero-length wait reported success on a cancelled run")
		}
	}

	srv, hits := serveStatus(t, http.StatusServiceUnavailable, nil)
	_, attempts, err := Do(ctx, srv.Client(), NewPacer(0), make([]time.Duration, 2), getFrom(srv))
	if err == nil {
		t.Fatal("a cancelled fetch returned no error")
	}
	if attempts != 1 || *hits > 1 {
		t.Errorf("%d requests over %d attempt(s), want one at most: a run being shut down does not spend its retries", *hits, attempts)
	}
}

// one pacer, several clients.

// TestTwoClientsOnOnePacerQueueBehindEachOther: two clients that declare the same host and
// are both built in one run each knew nothing of the other's pacer.
func TestTwoClientsOnOnePacerQueueBehindEachOther(t *testing.T) {
	const (
		host = "portal.broker.example"
		web  = 100 * time.Millisecond
		cp   = 140 * time.Millisecond
	)
	p := NewPacer(0)
	p.PaceHost(host, web)
	p.PaceHost(host, cp)
	ctx := context.Background()

	start := time.Now()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Wait(ctx, host)
		}()
	}
	wg.Wait()

	// Four turns at the longer of the two declarations: the first is free and
	// the other three are 140ms apart.
	if span := time.Since(start); span < 3*cp {
		t.Errorf("four requests from two clients spanned %v, want at least %v - "+
			"they were paced as though each client had the host to itself", span, 3*cp)
	}
}

// TestTheLongestDeclarationForAHostWins: politeness is a floor, in either build
// order.
func TestTheLongestDeclarationForAHostWins(t *testing.T) {
	for _, order := range [][]time.Duration{{300 * time.Millisecond, 400 * time.Millisecond},
		{400 * time.Millisecond, 300 * time.Millisecond}} {
		p := NewPacer(0)
		for _, d := range order {
			p.PaceHost("one.example", d)
		}
		if got := p.gapFor("one.example"); got != 400*time.Millisecond {
			t.Errorf("declared %v -> %v, want 400ms", order, got)
		}
	}
}

// TestOnePacerKeepsEveryHostsOwnInterval: a run holds the SEC at 150ms and the
// broker portal at 400ms, and one pacer must not make either the other.
func TestOnePacerKeepsEveryHostsOwnInterval(t *testing.T) {
	p := NewPacer(300 * time.Millisecond)
	p.PaceHost("www.sec.gov", 150*time.Millisecond)
	p.PaceHost("portal.broker.example", 400*time.Millisecond)

	for host, want := range map[string]time.Duration{
		"www.sec.gov":           150 * time.Millisecond,
		"portal.broker.example": 400 * time.Millisecond,
		"www.justetf.com":       300 * time.Millisecond, // nobody declared one: the default
	} {
		if got := p.gapFor(host); got != want {
			t.Errorf("%s paced at %v, want %v", host, got, want)
		}
	}
}

// TestAZeroValuePacerPacesNothingAndPanicsAtNothing: `&Pacer{}` compiles
// wherever a client is built, and used to panic on the first request.
func TestAZeroValuePacerPacesNothingAndPanicsAtNothing(t *testing.T) {
	p := &Pacer{}
	ctx := context.Background()
	start := time.Now()
	for range 20 {
		if !p.Wait(ctx, "one.example") {
			t.Fatal("wait reported a cancelled run on a live context")
		}
	}
	if waited := time.Since(start); waited > 100*time.Millisecond {
		t.Errorf("twenty requests through a zero-valued pacer took %v", waited)
	}
}

// TestANilPacerIsUnpacedRatherThanACrash: a client whose constructor never set
// one reaches the transport with nil.
func TestANilPacerIsUnpacedRatherThanACrash(t *testing.T) {
	srv, hits := serveStatus(t, http.StatusOK, nil)
	resp, _, err := Do(context.Background(), srv.Client(), nil, nil, getFrom(srv))
	if err != nil {
		t.Fatalf("fetchDo with no pacer: %v", err)
	}
	resp.Body.Close()
	if *hits != 1 {
		t.Errorf("%d requests, want 1", *hits)
	}
}

// TestThePacerForgetsHostsWhoseTurnHasPassed: next held one entry per host ever
// asked, which will not do for a pacer meant to outlive a stage.
func TestThePacerForgetsHostsWhoseTurnHasPassed(t *testing.T) {
	p := NewPacer(0)
	ctx := context.Background()
	for i := range pacerSweepAt + 10 {
		p.Wait(ctx, fmt.Sprintf("host-%d.example", i))
	}
	p.mu.Lock()
	n := len(p.next)
	p.mu.Unlock()
	if n > pacerSweepAt {
		t.Errorf("the pacer remembers %d hosts after %d were asked, want no more than %d",
			n, pacerSweepAt+10, pacerSweepAt)
	}

	// And what it forgets costs nothing: a host whose turn has passed owes no
	// wait, remembered or not.
	start := time.Now()
	if !p.Wait(ctx, "host-0.example") {
		t.Fatal("wait reported a cancelled run on a live context")
	}
	if waited := time.Since(start); waited > 50*time.Millisecond {
		t.Errorf("a swept host waited %v", waited)
	}
}
