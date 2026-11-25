// Package store is the atomic file write every pb output goes through, plus the
// lock and hash helpers the raw cache needs.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// TmpIgnore is the pattern of this package's scratch files: os.CreateTemp's ".tmp-"
// prefix, which covers lock.go's lockfiles too.
const TmpIgnore = ".tmp-*"

// WriteFileAtomic writes to a temporary in the destination directory and renames
// it into place. The temporary shares the directory to stay on one filesystem.
func WriteFileAtomic(p string, b []byte) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// Removed on every path; after a successful rename there is nothing at that
	// name and the failure is expected.
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	// Chmod through the descriptor rather than the name, so no other process can
	// substitute the path between the write and the mode.
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	// Flush before the rename, or a crash leaves the new name on a truncated file
	// whose bytes were only in the page cache.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, p); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir makes a completed rename durable. Best-effort: not every filesystem
// lets a directory be opened for sync.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// RelWithExt swaps an artefact path's extension. The stem is kept, so nav.csv
// becomes nav.json and the pair stays recognisable as the same document.
func RelWithExt(rel, ext string) string {
	if i := strings.LastIndexByte(rel, '.'); i >= 0 {
		if j := strings.LastIndexByte(rel, '/'); i > j {
			rel = rel[:i]
		}
	}
	return rel + ext
}

// SHA256Hex is the lowercase hex SHA-256 of b.
func SHA256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
