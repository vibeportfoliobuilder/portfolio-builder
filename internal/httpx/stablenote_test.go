package httpx

// stablenote_test.go - a refusal recorded on two runs must be the same bytes, or
// a fetch over an unchanged, still-refusing origin commits a second time.

import (
	"errors"
	"testing"
)

// TestAStreamResetReadsTheSameOnEveryRun: Go names the HTTP/2 stream in the
// error, and the number is per connection rather than a fact about the origin.
func TestAStreamResetReadsTheSameOnEveryRun(t *testing.T) {
	a := StableNote(errors.New(`Get "https://x/y": stream error: stream ID 5; INTERNAL_ERROR; received from peer`))
	b := StableNote(errors.New(`Get "https://x/y": stream error: stream ID 17; INTERNAL_ERROR; received from peer`))
	if a != b {
		t.Errorf("two resets of one origin read differently:\n%s\n%s", a, b)
	}
	if want := `Get "https://x/y": stream error: stream ID n; INTERNAL_ERROR; received from peer`; a != want {
		t.Errorf("note = %q, want %q", a, want)
	}
	if got := StableNote(errors.New("dial tcp: lookup x: no such host")); got != "dial tcp: lookup x: no such host" {
		t.Errorf("an error with no stream id was rewritten: %q", got)
	}
}
