package store

// locktakeover_test.go - the takeover: the moment one waiter decides a lockfile
// is dead, and a peer deciding the same about the same file.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAReleaseRemovesOnlyTheLockfileThisRunCreated: a run whose lockfile was
// taken from it must not delete whatever is at the name on its way out.
func TestAReleaseRemovesOnlyTheLockfileThisRunCreated(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notes.txt")
	unlock, err := lockFile(p)
	if err != nil {
		t.Fatal(err)
	}
	lp := lockPathFor(p)

	// What a peer's takeover leaves behind: this run's lockfile gone, and the
	// peer's own in its place.
	if err := os.Remove(lp); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lp, []byte("the peer's"), 0o644); err != nil {
		t.Fatal(err)
	}

	unlock()
	if _, err := os.Stat(lp); err != nil {
		t.Errorf("the lockfile of the run that holds the lock was removed by one that no longer does: %v", err)
	}
}

// TestAStaleTakeoverHandsTheLockToExactlyOneRun is the TOCTOU: stat-then-remove
// hands the lock to both waiters. Ten rounds, with a busy-wait starting gun.
func TestAStaleTakeoverHandsTheLockToExactlyOneRun(t *testing.T) {
	// A short poll, so the waiters that lose look again in milliseconds.
	old := lockPoll
	lockPoll = 2 * time.Millisecond
	defer func() { lockPoll = old }()

	const (
		rounds     = 10
		contenders = 8
	)
	for round := range rounds {
		p := filepath.Join(t.TempDir(), "universe.yaml")
		lp := lockPathFor(p)
		// The lockfile a killed run left behind, aged past lockStale without touching
		// the durations.
		if err := os.WriteFile(lp, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		aged := time.Now().Add(-time.Hour)
		if err := os.Chtimes(lp, aged, aged); err != nil {
			t.Fatal(err)
		}
		// The instant every waiter starts. A WaitGroup is not a starting gun: the
		// scheduler would run the takeovers one whole at a time.
		gun := time.Now().Add(2 * time.Millisecond)

		var (
			held  atomic.Int32
			start sync.WaitGroup
			done  sync.WaitGroup
		)
		start.Add(1)
		errs := make([]error, contenders)
		clash := make([]bool, contenders)
		for i := range contenders {
			done.Add(1)
			go func() {
				defer done.Done()
				start.Wait()
				for time.Now().Before(gun) {
				}
				unlock, err := lockFile(p)
				if err != nil {
					errs[i] = err
					return
				}
				clash[i] = held.Add(1) != 1
				time.Sleep(time.Millisecond)
				held.Add(-1)
				unlock()
			}()
		}
		start.Done()
		done.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: waiter %d never got the lock a killed run left behind: %v", round, i, err)
			}
		}
		for i, both := range clash {
			if both {
				t.Fatalf("round %d: waiter %d held the lock while a peer held it too - both took over the same stale lockfile", round, i)
			}
		}
	}
}

// TestATakeoverLeavesNoFileTheSnapshotWouldCommit: the takeover writes through
// a temporary, so a crash before the rename leaves that file in the snapshot.
func TestATakeoverLeavesNoFileTheSnapshotWouldCommit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "universe.yaml")
	lp := lockPathFor(p)
	if err := os.WriteFile(lp, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	aged := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lp, aged, aged); err != nil {
		t.Fatal(err)
	}

	unlock, err := lockFile(p)
	if err != nil {
		t.Fatal(err)
	}
	unlock()

	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	prefix := strings.TrimSuffix(TmpIgnore, "*")
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), prefix) {
			t.Errorf("the takeover left %q behind, which the ignore pattern %q does not cover", e.Name(), TmpIgnore)
		}
	}
}
