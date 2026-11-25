// Package httpx is the transport layer: per-host pacing, retries, and the
// failures that are about the moment rather than the document.
package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UserAgent is what every request to a publisher says it is, in one spelling so
// an origin's logs and allow-lists see one client.
const UserAgent = "etfcompare/1.0 (official-source snapshot)"

// Pacer keeps requests to any one host at least an interval apart, per host: each
// client declares its own origin's interval and --host-delay covers the rest.
type Pacer struct {
	mu   sync.Mutex
	next map[string]time.Time
	// hosts are the intervals declared per host, keyed lower-case like next, as
	// a host name is case-insensitive. A host with no entry is paced at min.
	hosts map[string]time.Duration
	min   time.Duration
}

func NewPacer(minGap time.Duration) *Pacer {
	return &Pacer{next: map[string]time.Time{}, hosts: map[string]time.Duration{}, min: minGap}
}

// theRunPacer is the pacer every client in this process shares, because
// politeness is owed by the PROCESS to an origin, not by each client.
var theRunPacer = NewPacer(0)

// RunPacer is the pacer every client in this process shares. A caller still
// declares what its own origin is owed, with PaceHost.
func RunPacer() *Pacer { return theRunPacer }

// PaceHost declares the interval owed to one host, and the LONGEST declaration
// wins: politeness is a floor, so 300ms and 400ms resolve to 400.
func (p *Pacer) PaceHost(host string, gap time.Duration) {
	host = strings.ToLower(host)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hosts == nil {
		p.hosts = map[string]time.Duration{}
	}
	if cur, ok := p.hosts[host]; !ok || gap > cur {
		p.hosts[host] = gap
	}
}

// PaceDefault declares the interval owed to a host nobody named, under the same
// longest-wins rule; it is how --host-delay reaches the shared pacer.
func (p *Pacer) PaceDefault(gap time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if gap > p.min {
		p.min = gap
	}
}

// gapFor is the interval owed to one host. The caller holds the lock.
func (p *Pacer) gapFor(host string) time.Duration {
	if d, ok := p.hosts[host]; ok {
		return d
	}
	return p.min
}

// pacerSweepAt is how many hosts may accumulate before Wait drops the ones whose
// turn has passed, which is free: such an entry imposes no wait.
const pacerSweepAt = 1024

// Wait blocks until this host's turn, false if the run was cancelled first. The
// turn is reserved under the lock and slept outside it, or the gap goes global.
func (p *Pacer) Wait(ctx context.Context, host string) bool {
	host = strings.ToLower(host)
	p.mu.Lock()
	now := time.Now()
	turn := now
	if t, ok := p.next[host]; ok && t.After(turn) {
		turn = t
	}
	// `&Pacer{}` compiles anywhere a client is built, so a zero-valued pacer is
	// an unpaced one rather than a panic on the first request.
	if p.next == nil {
		p.next = map[string]time.Time{}
	}
	if len(p.next) > pacerSweepAt {
		for h, t := range p.next {
			if h != host && !t.After(now) {
				delete(p.next, h)
			}
		}
	}
	p.next[host] = turn.Add(p.gapFor(host))
	p.mu.Unlock()

	// SleepCtx answers both halves: a turn already due is not a wait, and a
	// cancelled run is not a turn.
	return SleepCtx(ctx, time.Until(turn))
}

// Retrying a request that failed for a reason that is not about the document:
// Invesco's cold cache entries shed load with 503 and answer a minute later.

// fetchRetryStatus are the codes worth asking again about - "not now" rather than
// "not this". 404 is not among them: retrying it cost ~172 requests per fund walk.
var fetchRetryStatus = map[int]bool{
	http.StatusTooManyRequests:     true, // 429
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
}

// Option is one call's amendment to that list: the ETF document download adds 404,
// because J.P. Morgan's KID address flaps between runs, and justetf adds 403.
type Option func(*retryPolicy)

// retryPolicy is the retry rule as one call sees it.
type retryPolicy struct {
	// also are the statuses this caller added to fetchRetryStatus, a slice
	// rather than a set because it holds one entry in the only use there is.
	also []int
}

// retry reports whether this status is worth asking again about on this call.
func (p retryPolicy) retry(code int) bool {
	return fetchRetryStatus[code] || slices.Contains(p.also, code)
}

// RetryAlso names statuses this caller wants retried on top of the shared list;
// see Option for why this is asked for rather than assumed.
func RetryAlso(status ...int) Option {
	return func(p *retryPolicy) { p.also = append(p.also, status...) }
}

// IsGone reports the two statuses that mean an address is no longer an address, 404
// and 410; a 500 or a 403 is not here, the document still being where it was.
func IsGone(code int) bool {
	return code == http.StatusNotFound || code == http.StatusGone
}

// Challenged reports whether a response is an edge's interstitial rather than
// the publisher's answer: Cloudflare serves one as 403 with cf-mitigated.
func Challenged(resp *http.Response) bool {
	return strings.EqualFold(strings.TrimSpace(resp.Header.Get("cf-mitigated")), "challenge")
}

// DefaultBackoff is the wait before each retry, and its length is how many
// retries there are. A fresh slice, so no caller can shrink another's schedule.
func DefaultBackoff() []time.Duration { return []time.Duration{2 * time.Second, 6 * time.Second} }

// fetchRetryAfterMax caps how long a Retry-After is honoured for: a server
// asking for an hour is asking for more than a snapshot run can give.
const fetchRetryAfterMax = 30 * time.Second

// Do sends the request, retrying the failures that are about the moment, and returns the
// attempts made. A nil hc means http.DefaultClient, nil backoff DefaultBackoff, empty none.
func Do(ctx context.Context, hc *http.Client, pacer *Pacer, backoff []time.Duration, build func() (*http.Request, error), opts ...Option) (*http.Response, int, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	if backoff == nil {
		backoff = DefaultBackoff()
	}
	var policy retryPolicy
	for _, o := range opts {
		o(&policy)
	}
	for attempt := 0; ; attempt++ {
		req, err := build()
		if err != nil {
			return nil, attempt + 1, err
		}
		// nil is unpaced rather than a panic, because a caller that does not
		// want pacing is a legitimate one.
		if pacer != nil && !pacer.Wait(ctx, req.URL.Host) {
			return nil, attempt + 1, ctx.Err()
		}
		resp, err := hc.Do(req)

		last := attempt >= len(backoff)
		switch {
		case err != nil:
			// A transport failure is never about the document, so it is always worth one
			// more try. Our own shutdown needs no case: the wait below notices.
			if last {
				return nil, attempt + 1, err
			}
		case policy.retry(resp.StatusCode) || Challenged(resp):
			if last {
				return resp, attempt + 1, nil
			}
			wait, canWait := retryAfter(resp, backoff[attempt])
			if !canWait {
				// The origin named a moment further off than this run can wait for, so hand
				// the refusal back rather than asking three times inside eight seconds.
				return resp, attempt + 1, nil
			}
			// The body of a refusal is not wanted, but it has to be drained and
			// closed or the connection cannot be reused.
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
			resp.Body.Close()
			if !SleepCtx(ctx, wait) {
				return nil, attempt + 1, ctx.Err()
			}
			continue
		default:
			return resp, attempt + 1, nil
		}

		if !SleepCtx(ctx, backoff[attempt]) {
			return nil, attempt + 1, ctx.Err()
		}
	}
}

// retryAfter reads the server's own "come back when", falling back to our
// backoff. ok is false when it asked for longer than the cap, which stops the retry.
func retryAfter(resp *http.Response, fallback time.Duration) (time.Duration, bool) {
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if v == "" {
		return fallback, true
	}
	if secs, err := strconv.Atoi(v); err == nil {
		switch d := time.Duration(secs) * time.Second; {
		case d < 0:
			return fallback, true
		case d > fetchRetryAfterMax:
			return 0, false
		default:
			return d, true
		}
	}
	if when, err := http.ParseTime(v); err == nil {
		switch d := time.Until(when); {
		case d > fetchRetryAfterMax:
			return 0, false
		case d < 0:
			return 0, true
		default:
			return d, true
		}
	}
	return fallback, true
}

// Fetch is one request through Do that must answer 200, its body ReadCapped at limit; seen,
// when set, reads the 200's headers. Refusals: "<who>: <url> (n attempts): err", "<who>: <url>: http N".
func Fetch(ctx context.Context, hc *http.Client, pacer *Pacer, backoff []time.Duration, who, url string, limit int64, build func() (*http.Request, error), seen func(*http.Response)) ([]byte, error) {
	resp, attempts, err := Do(ctx, hc, pacer, backoff, build)
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", who, WithAttempts(url, attempts), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s: http %d", who, url, resp.StatusCode)
	}
	b, err := ReadCapped(resp.Body, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: reading %s: %w", who, url, err)
	}
	if seen != nil {
		seen(resp)
	}
	return b, nil
}

// Exchange is one request through Do with its body ReadCapped at limit whatever the status;
// judge sees both before the body closes. Refusals: "<who>: <what>: err (n attempts)".
func Exchange(ctx context.Context, hc *http.Client, pacer *Pacer, backoff []time.Duration, who, what string, limit int64,
	build func() (*http.Request, error), judge func(*http.Response, []byte) ([]byte, error), opts ...Option) ([]byte, error) {
	resp, attempts, err := Do(ctx, hc, pacer, backoff, build, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %s", who, what, WithAttempts(err.Error(), attempts))
	}
	defer resp.Body.Close()
	raw, err := ReadCapped(resp.Body, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", who, what, err)
	}
	return judge(resp, raw)
}

// ReadCapped reads a body to its end, refusing one longer than limit rather than cutting it
// there: a document truncated at its ceiling is a wrong answer, not a smaller one.
func ReadCapped(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err == nil && int64(len(b)) > limit {
		return b[:limit], fmt.Errorf("document is larger than the %s ceiling", ceilingText(limit))
	}
	return b, err
}

// ceilingText names a size limit: whole mebibytes as "N MB", anything else in bytes.
func ceilingText(n int64) string {
	if n >= 1<<20 && n%(1<<20) == 0 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d-byte", n)
}

// SleepCtx waits, false if the run was cancelled while it did. Both guards are
// load-bearing: a select over two ready channels picks at RANDOM.
func SleepCtx(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// clipAt is how much of a refusal's body an error carries.
const clipAt = 200

// Clip is at most the first clipAt bytes of a refusal's body, marked where cut:
// enough to recognise the answer, not so much that a long one floods a log line.
func Clip(body string) string {
	if len(body) > clipAt {
		return body[:clipAt] + "..."
	}
	return body
}

// Snippet is Clip over the trimmed body, naming an empty one rather than leaving
// nothing after the colon.
func Snippet(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "(an empty body)"
	}
	return Clip(body)
}

// ErrorReport reads a body that is NOTHING BUT a JSON error report - every key one
// of error, message, reason, type, status, statusCode - as "type: message".
func ErrorReport(b []byte) (string, bool) {
	if len(b) == 0 || b[0] != '{' {
		return "", false
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(b, &probe) != nil || len(probe) == 0 {
		return "", false
	}
	for k := range probe {
		switch k {
		case "error", "message", "reason", "type", "status", "statusCode":
		default:
			return "", false
		}
	}
	var v struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Reason  string `json:"reason"`
		Type    string `json:"type"`
	}
	if json.Unmarshal(b, &v) != nil {
		return "", false
	}
	for _, cand := range []string{v.Error, v.Message, v.Reason} {
		msg := strings.TrimSpace(cand)
		if msg == "" {
			continue
		}
		// The report's own classification in front of its sentence:
		// IDENTIFICATION_PROBLEM is not deducible from the prose.
		if t := strings.TrimSpace(v.Type); t != "" && !strings.EqualFold(t, msg) {
			return t + ": " + msg, true
		}
		return msg, true
	}
	return "", false
}

// StatusError is a refusal carried with its body, printed through Snippet. Source names
// the asking package and prefixes the message; Path, when set, is what was asked for.
type StatusError struct {
	Source string
	Status int
	Path   string
	Body   string
}

func (e *StatusError) Error() string {
	body := Snippet(e.Body)
	if e.Path == "" {
		return fmt.Sprintf("%s: http %d: %s", e.Source, e.Status, body)
	}
	return fmt.Sprintf("%s: %s: http %d: %s", e.Source, e.Path, e.Status, body)
}

// StableNote is a transport error as a note that repeats: an HTTP/2 stream id
// changes per connection, and a refusal recorded twice must record the same bytes.
func StableNote(err error) string {
	return streamID.ReplaceAllString(err.Error(), "stream ID n")
}

var streamID = regexp.MustCompile(`stream ID \d+`)

// WithAttempts records that a failure survived being retried, so a manifest tells
// a source that was down from one asked at a bad moment.
func WithAttempts(note string, attempts int) string {
	if attempts <= 1 {
		return note
	}
	return fmt.Sprintf("%s (%d attempts)", note, attempts)
}
