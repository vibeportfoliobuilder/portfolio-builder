// lock.go - the exclusive lock a read-modify-write of a SHARED snapshot file
// needs. O_EXCL create is the lock, and the lockfile is ".tmp-<name>.lock".

package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// var rather than const so the lock tests can shorten them: otherwise one test
// sleeps for the whole of lockWait and another cannot run at all.
var (
	lockPoll  = 20 * time.Millisecond
	lockWait  = 5 * time.Second
	lockStale = 2 * time.Minute
)

// lockPathFor is the lockfile that guards p.
func lockPathFor(p string) string {
	return filepath.Join(filepath.Dir(p), ".tmp-"+filepath.Base(p)+".lock")
}

// lockFile serialises writers of p against each other - in this process or in
// another one - and returns the function that releases the lock.
func lockFile(p string) (func(), error) {
	return LockPath(lockPathFor(p), lockWait, lockStale)
}

// LockPath is the lock itself and the ONE implementation, exported for
// internal/cache. A holder that outlives stale has the lock taken off it.
func LockPath(lockfile string, wait, stale time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(lockfile), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(lockfile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			mine := stamp(f)
			_ = f.Close()
			return func() { release(lockfile, mine) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("lock %s: %w", lockfile, err)
		}
		// A lock older than any critical section belongs to a process killed between
		// creating it and removing it. Age is read from the file, not remembered.
		if fi, statErr := os.Stat(lockfile); statErr == nil && time.Since(fi.ModTime()) > stale {
			if mine, ok := takeOverStale(lockfile, fi, stale); ok {
				return func() { release(lockfile, mine) }, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock %s: held by another run after %s", lockfile, wait)
		}
		time.Sleep(lockPoll)
	}
}

// takeOverStale replaces the dead lockfile with one of this run's own and
// reports whether this run holds the lock. A RENAME, so the name never vanishes.
func takeOverStale(lockfile string, judged os.FileInfo, stale time.Duration) (*owner, bool) {
	claims, ok := claimDead(lockfile, judged, stale)
	defer func() {
		for _, c := range claims {
			_ = os.Remove(c)
		}
	}()
	if !ok {
		return nil, false
	}
	// The claim keeps every peer off the judged file, which may have been replaced
	// before it was made; a lockfile is written only by the run that creates it, so its mtime is its own.
	if fi, err := os.Stat(lockfile); err != nil || !os.SameFile(fi, judged) || !fi.ModTime().Equal(judged.ModTime()) {
		return nil, false
	}
	// Named ".tmp-lock-*" so that a crash before the rename leaves a file the
	// snapshot's ignore rule already covers (TmpIgnore in store.go).
	f, err := os.CreateTemp(filepath.Dir(lockfile), ".tmp-lock-*")
	if err != nil {
		return nil, false
	}
	mine := stamp(f)
	_ = f.Close()
	if mine == nil {
		_ = os.Remove(f.Name())
		return nil, false
	}
	if err := os.Rename(f.Name(), lockfile); err != nil {
		_ = os.Remove(f.Name())
		return nil, false
	}
	return mine, true
}

// claimDead makes this run the only one that may replace the judged lockfile: an O_EXCL claim
// named after its mtime, and the next generation when a claim outlived stale (its maker died).
func claimDead(lockfile string, judged os.FileInfo, stale time.Duration) ([]string, bool) {
	var claims []string
	for gen := 0; ; gen++ {
		c := fmt.Sprintf("%s.%d.claim%d", lockfile, judged.ModTime().UnixNano(), gen)
		f, err := os.OpenFile(c, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return append(claims, c), true
		}
		if fi, statErr := os.Stat(c); !os.IsExist(err) || statErr != nil || time.Since(fi.ModTime()) <= stale {
			return nil, false
		}
		claims = append(claims, c)
	}
}

// owner is how a run recognises its own lockfile: the identity the filesystem
// gave it and a random token written into it. The token is needed because
// Linux reuses a removed file's inode at once, so a peer's lockfile at the same
// name can have the very identity this run's had.
type owner struct {
	fi    os.FileInfo
	token []byte
}

// stamp writes a fresh token into the just-created lockfile f. It returns nil
// when the file cannot be stat'ed; a token that cannot be made or written is
// left empty, and the identity alone then decides.
func stamp(f *os.File) *owner {
	var tok []byte
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		tok = []byte(hex.EncodeToString(b))
		if _, err := f.Write(tok); err != nil {
			tok = nil
		}
	}
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	return &owner{fi: fi, token: tok}
}

// release removes the lockfile only while it is still the one this run
// installed, or a run whose lock was taken would delete the holder's.
func release(lockfile string, mine *owner) {
	if mine == nil {
		_ = os.Remove(lockfile)
		return
	}
	if fi, err := os.Stat(lockfile); err != nil || !os.SameFile(fi, mine.fi) {
		return
	}
	if mine.token != nil {
		if got, err := os.ReadFile(lockfile); err != nil || !bytes.Equal(got, mine.token) {
			return
		}
	}
	_ = os.Remove(lockfile)
}
