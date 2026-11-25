package cache

// save_test.go - Save across two openers. It used to write the loaded index
// straight over index.json, so whichever saved last dropped the other's entries.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
)

func openCacheAt(t *testing.T, root string) *Cache {
	t.Helper()
	c, err := OpenCache(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestTwoOpenersDoNotLoseEachOthersEntries is the defect itself: a fetch run by hand while
// a build is in flight, and every wave by construction.
func TestTwoOpenersDoNotLoseEachOthersEntries(t *testing.T) {
	root := t.TempDir()
	a := openCacheAt(t, root)
	b := openCacheAt(t, root)

	if _, err := a.Put("funds/IE00AAAAAAAA/nav.csv", "https://a.test/n", []byte("date,nav\n2026-08-06,10\n"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Put("funds/IE00BBBBBBBB/nav.csv", "https://b.test/n", []byte("date,nav\n2026-08-06,20\n"), nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}

	reopened := openCacheAt(t, root)
	for _, key := range []string{"funds/IE00AAAAAAAA/nav.csv", "funds/IE00BBBBBBBB/nav.csv"} {
		if _, _, ok := reopened.Get(key); !ok {
			t.Errorf("%s is not in the index that landed; its blob is orphaned and the next run re-downloads it", key)
		}
	}
}

// TestAStaleCopyDoesNotRevertAPeersFetch is why the merge is per key: B's older
// copy of an entry it never touched must not overwrite A's refetch.
func TestAStaleCopyDoesNotRevertAPeersFetch(t *testing.T) {
	root := t.TempDir()
	const key = "funds/IE00CCCCCCCC/nav.csv"

	seed := openCacheAt(t, root)
	if _, err := seed.Put(key, "https://c.test/n", []byte("date,nav\n2026-08-01,9\n"), nil); err != nil {
		t.Fatal(err)
	}
	if err := seed.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}

	// B opens while the old entry is current, and never touches that key.
	b := openCacheAt(t, root)

	a := openCacheAt(t, root)
	fresh, err := a.Put(key, "https://c.test/n", []byte("date,nav\n2026-08-06,10\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}

	if _, err := b.Put("funds/IE00DDDDDDDD/nav.csv", "https://d.test/n", []byte("date,nav\n2026-08-06,30\n"), nil); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}

	reopened := openCacheAt(t, root)
	if _, e, _ := reopened.Get(key); e.SHA256 != fresh {
		t.Errorf("%s addresses %q, want the peer's fresh blob %q", key, e.SHA256, fresh)
	}
	if _, _, ok := reopened.Get("funds/IE00DDDDDDDD/nav.csv"); !ok {
		t.Error("the saver's own entry was dropped")
	}
}

// TestSaveOfALoadAssertsNothingNew: Save(Load()) writes out what Put recorded
// and must stay a no-op for every key this process only READ.
func TestSaveOfALoadAssertsNothingNew(t *testing.T) {
	root := t.TempDir()
	const key = "funds/IE00EEEEEEEE/nav.csv"

	seed := openCacheAt(t, root)
	if _, err := seed.Put(key, "https://e.test/n", []byte("date,nav\n2026-08-01,9\n"), nil); err != nil {
		t.Fatal(err)
	}
	if err := seed.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}

	b := openCacheAt(t, root) // holds the old entry, touches nothing

	a := openCacheAt(t, root)
	fresh, err := a.Put(key, "https://e.test/n", []byte("date,nav\n2026-08-06,10\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}

	if err := b.Save(b.Load()); err != nil {
		t.Fatal(err)
	}
	if _, e, _ := openCacheAt(t, root).Get(key); e.SHA256 != fresh {
		t.Errorf("Save(Load()) reasserted an entry it had only read: %q, want %q", e.SHA256, fresh)
	}
}

// TestAHeldLockIsRefusedRatherThanClobbered: without the lock two Saves
// interleave between the read and the rename and the loss comes back.
func TestAHeldLockIsRefusedRatherThanClobbered(t *testing.T) {
	old := indexLockWait
	indexLockWait = 50 * time.Millisecond
	defer func() { indexLockWait = old }()

	c := openCache(t)
	if _, err := c.Put("funds/IE00FFFFFFFF/nav.csv", "https://f.test/n", []byte("x"), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(c.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(c.Dir(), indexLockName)
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(core.NewManifest()); err == nil {
		t.Fatal("Save went ahead through a held lock")
	}
	if _, err := os.Stat(filepath.Join(c.Dir(), cacheIndexFile)); !os.IsNotExist(err) {
		t.Errorf("Save wrote the index anyway: %v", err)
	}
	// The peer's lock is still the peer's: a refusal must not release it.
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("the lock was removed by the run that failed to take it: %v", err)
	}
}

// TestAStaleLockIsTakenOver: Save's critical section is milliseconds, so a
// lockfile minutes old belongs to a killed process.
func TestAStaleLockIsTakenOver(t *testing.T) {
	oldWait, oldStale := indexLockWait, indexLockStale
	indexLockWait, indexLockStale = 50*time.Millisecond, time.Millisecond
	defer func() { indexLockWait, indexLockStale = oldWait, oldStale }()

	c := openCache(t)
	if _, err := c.Put("funds/IE00GGGGGGGG/nav.csv", "https://g.test/n", []byte("x"), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(c.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(c.Dir(), indexLockName)
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := c.Save(core.NewManifest()); err != nil {
		t.Fatalf("a lock left by a killed run wedged the cache: %v", err)
	}
	// And the lock is released by the run that took it, so the next Save is not
	// itself waiting on a stale file.
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the lockfile outlived a successful Save: %v", err)
	}
}

// TestTheLockNeverReachesTheSnapshot: it lives in the cache directory, which
// .gitignore excludes wholesale.
func TestTheLockNeverReachesTheSnapshot(t *testing.T) {
	c := openCache(t)
	if got := filepath.Dir(filepath.Join(c.Dir(), indexLockName)); got != c.Dir() {
		t.Errorf("the lock is at %s, outside the ignored cache directory", got)
	}
}

// TestSaveKeepsThisRunsIndexWhenTheFileOnDiskIsGone: an unreadable index.json
// would empty the merge's base and orphan every blob this run did not fetch.
func TestSaveKeepsThisRunsIndexWhenTheFileOnDiskIsGone(t *testing.T) {
	loaded := []string{
		"funds/IE0000000106/nav.csv",
		"funds/IE0000000107/nav.csv",
		"funds/IE0000000108/nav.csv",
	}
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, path string)
	}{
		{"deleted", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
		{"truncated", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`{"tool":"etfcompare","entr`), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			seed := openCacheAt(t, root)
			for i, key := range loaded {
				if _, err := seed.Put(key, "https://h.test/n", []byte{'n', byte('0' + i)}, nil); err != nil {
					t.Fatal(err)
				}
			}
			if err := seed.Save(core.NewManifest()); err != nil {
				t.Fatal(err)
			}

			c := openCacheAt(t, root) // holds all three, touches none of them
			tc.damage(t, filepath.Join(c.Dir(), cacheIndexFile))
			if _, err := c.Put("funds/IE0000000109/nav.csv", "https://h.test/n", []byte("n9"), nil); err != nil {
				t.Fatal(err)
			}
			if err := c.Save(core.NewManifest()); err != nil {
				t.Fatal(err)
			}

			reopened := openCacheAt(t, root)
			for _, key := range loaded {
				if _, _, ok := reopened.Get(key); !ok {
					t.Errorf("%s is not in the index that landed; its blob is orphaned and the next run re-downloads it", key)
				}
			}
			if _, _, ok := reopened.Get("funds/IE0000000109/nav.csv"); !ok {
				t.Error("the entry this run fetched was dropped as well")
			}
		})
	}
}

// TestSaveOfALoadSurvivesADamagedIndex is the same loss through the form the
// stages call: nothing dirty to layer, so an empty index would land.
func TestSaveOfALoadSurvivesADamagedIndex(t *testing.T) {
	root := t.TempDir()
	const key = "funds/IE0000000110/nav.csv"

	seed := openCacheAt(t, root)
	if _, err := seed.Put(key, "https://h.test/n", []byte("n10"), nil); err != nil {
		t.Fatal(err)
	}
	if err := seed.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}

	c := openCacheAt(t, root)
	if err := os.Remove(filepath.Join(c.Dir(), cacheIndexFile)); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(c.Load()); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := openCacheAt(t, root).Get(key); !ok {
		t.Errorf("%s was dropped by a Save(Load()) that had no file to merge onto", key)
	}
}

// TestTheIndexLockIsReleasedOnlyByTheRunThatStillHoldsIt: a run whose lockfile
// was taken from it must not remove whatever is at that name.
func TestTheIndexLockIsReleasedOnlyByTheRunThatStillHoldsIt(t *testing.T) {
	c := openCache(t)
	if err := os.MkdirAll(c.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	unlock, err := c.lockIndex()
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(c.Dir(), indexLockName)
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("the peer's"), 0o644); err != nil {
		t.Fatal(err)
	}

	unlock()
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("the lockfile of the run holding the index lock was removed by one that no longer holds it: %v", err)
	}
}
