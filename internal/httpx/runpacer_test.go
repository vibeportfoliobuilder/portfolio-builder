package httpx

// runpacer_test.go - the process's shared pacer, and the default gap that is
// how -host-delay reaches it.

import (
	"testing"
	"time"
)

// TestTheDefaultGapTakesTheLongestDeclaration: the stages reach the shared pacer
// one after another, so which spoke first must not decide an issuer's pacing.
func TestTheDefaultGapTakesTheLongestDeclaration(t *testing.T) {
	for _, order := range [][]time.Duration{
		{500 * time.Millisecond, 1500 * time.Millisecond},
		{1500 * time.Millisecond, 500 * time.Millisecond},
	} {
		p := NewPacer(0)
		for _, d := range order {
			p.PaceDefault(d)
		}
		if got := p.gapFor("issuer.example"); got != 1500*time.Millisecond {
			t.Errorf("declared %v -> %v, want 1.5s", order, got)
		}
	}
}

// TestTheDefaultGapDoesNotOverruleAHostsOwn: --host-delay is for the hosts
// nothing else names, so it must not slow or speed one with its own client.
func TestTheDefaultGapDoesNotOverruleAHostsOwn(t *testing.T) {
	p := NewPacer(0)
	p.PaceHost("www.sec.gov", 150*time.Millisecond)
	p.PaceHost("portal.broker.example", 400*time.Millisecond)
	p.PaceDefault(1500 * time.Millisecond)

	for host, want := range map[string]time.Duration{
		"www.sec.gov":           150 * time.Millisecond,
		"portal.broker.example": 400 * time.Millisecond,
		"issuer.example":        1500 * time.Millisecond, // nobody declared one
	} {
		if got := p.gapFor(host); got != want {
			t.Errorf("%s paced at %v, want %v", host, got, want)
		}
	}
}

// TestTheRunPacerIsOneInstance: two commands in one process that ask for it
// must get the same clock, or each keeps a gap the origin never sees.
func TestTheRunPacerIsOneInstance(t *testing.T) {
	// Through variables rather than inline, because staticcheck reads
	// `RunPacer() != RunPacer()` as identical expressions (SA4000).
	first, second := RunPacer(), RunPacer()
	if first != second {
		t.Fatal("RunPacer returned two different pacers; every client in a process shares one")
	}
	if first == nil {
		t.Fatal("RunPacer returned nil, which is the unpaced case and not a shared clock")
	}
}
