package eulerpool

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// Cache stores raw bodies by request key; the client judges freshness from fetchedAt.
type Cache interface {
	Get(key string) (body []byte, fetchedAt time.Time, ok bool)
	Put(key string, body []byte, fetchedAt time.Time) error
}

// Defaults for DiskCache: a 200 MB cap and a 30-day age after which entries are pruned.
const (
	DefaultCacheBytes  = 200 << 20
	DefaultCacheMaxAge = 30 * 24 * time.Hour
)

// pruneEvery is how many Puts pass between two directory sweeps.
const pruneEvery = 64

// DefaultCacheDir is os.UserCacheDir()/pb/eulerpool, outside any repository.
func DefaultCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "pb", "eulerpool"), nil
}

// DiskCache is a flat directory of bodies named by the SHA-256 of their key, capped and aged.
type DiskCache struct {
	Dir      string
	MaxBytes int64
	MaxAge   time.Duration
	Now      func() time.Time

	mu   sync.Mutex
	puts int
}

// NewDiskCache opens a cache at dir with the default cap and age.
func NewDiskCache(dir string) *DiskCache {
	return &DiskCache{Dir: dir, MaxBytes: DefaultCacheBytes, MaxAge: DefaultCacheMaxAge}
}

func (d *DiskCache) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *DiskCache) path(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(d.Dir, hex.EncodeToString(h[:])+".body")
}

// Get returns a body and the instant it was stored (the file's mtime).
func (d *DiskCache) Get(key string) ([]byte, time.Time, bool) {
	p := d.path(key)
	st, err := os.Stat(p)
	if err != nil {
		return nil, time.Time{}, false
	}
	if d.MaxAge > 0 && d.now().Sub(st.ModTime()) > d.MaxAge {
		_ = os.Remove(p)
		return nil, time.Time{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, time.Time{}, false
	}
	return b, st.ModTime(), true
}

// Put writes a body atomically, stamps it with fetchedAt and prunes now and then.
func (d *DiskCache) Put(key string, body []byte, fetchedAt time.Time) error {
	p := d.path(key)
	if err := store.WriteFileAtomic(p, body); err != nil {
		return err
	}
	if err := os.Chtimes(p, fetchedAt, fetchedAt); err != nil {
		return err
	}
	d.mu.Lock()
	d.puts++
	due := d.puts%pruneEvery == 1
	d.mu.Unlock()
	if due {
		return d.Prune()
	}
	return nil
}

// Prune drops entries older than MaxAge, then the oldest until the total is under 90% of MaxBytes.
func (d *DiskCache) Prune() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	var all []entry
	var total int64
	err := filepath.WalkDir(d.Dir, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || filepath.Ext(p) != ".body" {
			return nil
		}
		info, err := de.Info()
		if err != nil {
			return nil
		}
		if d.MaxAge > 0 && d.now().Sub(info.ModTime()) > d.MaxAge {
			_ = os.Remove(p)
			return nil
		}
		all = append(all, entry{p, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if d.MaxBytes <= 0 || total <= d.MaxBytes {
		return nil
	}
	slices.SortFunc(all, func(a, b entry) int { return a.mod.Compare(b.mod) })
	for _, e := range all {
		if total <= d.MaxBytes*9/10 {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
		}
	}
	return nil
}
