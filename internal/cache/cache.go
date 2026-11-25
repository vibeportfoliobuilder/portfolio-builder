// Package cache holds the raw downloads, content-addressed under <repo>/cache/,
// which .gitignore excludes: committing transfer bookkeeping produced the diff.
package cache

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

const (
	cacheDir       = "cache"
	cacheIndexFile = "index.json"
)

// Cache is the raw-download store under one snapshot repository. Root is the
// REPOSITORY root, and the mutex is there because Put is called per host.
type Cache struct {
	Root string
	// NoServe is what --no-cache means: Get misses for every key - no bytes and no
	// entry, since a 304 would answer "use your copy" - while Put goes on working.
	NoServe bool
	// Persist merges every Put into index.json at once, for a holder with no end of run to
	// Save at: an agent's live tools, whose sessions outlive any one call.
	Persist bool
	mu      sync.Mutex
	index   core.Manifest
	// dirty names the keys THIS process has something new to say about, read only
	// by Save: a peer that saved meanwhile knows more about a key this run never touched.
	dirty map[string]bool
}

// OpenCache reads the index for the repository at root, or starts an empty one.
// It creates nothing: a read-only run must leave no cache directory behind.
func OpenCache(root string) (*Cache, error) {
	if root == "" {
		return nil, fmt.Errorf("empty repository path")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	c := &Cache{Root: abs, index: core.NewManifest(), dirty: map[string]bool{}}
	if m, ok := readIndexFile(abs); ok {
		c.index = m
	}
	return c, nil
}

// readIndexFile reads <root>/cache/index.json and reports false for every way it
// can fail: an unreadable index is a MISS, since the blobs are addressed by content.
func readIndexFile(root string) (core.Manifest, bool) {
	b, err := os.ReadFile(filepath.Join(root, cacheDir, cacheIndexFile))
	if err != nil {
		return core.Manifest{}, false
	}
	var m core.Manifest
	if json.Unmarshal(b, &m) != nil || m.Entries == nil {
		return core.Manifest{}, false
	}
	return pruneAliasKeys(m), true
}

// pruneAliasKeys drops the entries an older revision wrote under the STORED
// format's path. An entry with no Kind sharing a path with one that has a Kind is an alias.
func pruneAliasKeys(m core.Manifest) core.Manifest {
	named := map[string]bool{}
	for k, e := range m.Entries {
		if e.Kind != "" {
			named[store.RelWithExt(k, "")] = true
		}
	}
	for k, e := range m.Entries {
		if e.Kind == "" && named[store.RelWithExt(k, "")] {
			delete(m.Entries, k)
		}
	}
	return m
}

// Dir is the cache directory: <repo>/cache.
func (c *Cache) Dir() string { return filepath.Join(c.Root, cacheDir) }

// blobPath is where the bytes with this sha live. The two-character fan-out
// keeps any one directory to a few hundred entries, which ext4 and APFS need.
func (c *Cache) blobPath(sha string) string {
	return filepath.Join(c.Dir(), sha[:2], sha)
}

// Put stores body against key - the artefact's OWN Rel() - and returns the sha.
// The bytes are stored EXACTLY as given, so the caller must Canonicalise first.
func (c *Cache) Put(key, url string, body []byte, resp *http.Response) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("cache put: empty key")
	}
	// An empty body is a failed transfer. Storing it would put an entry in the
	// index whose blob is nothing, read back next run as a valid copy.
	if len(body) == 0 {
		return "", fmt.Errorf("cache put %s: empty body", key)
	}
	sha := store.SHA256Hex(body)
	p := c.blobPath(sha)

	// Content addressing means an existing blob with this name IS this body, so
	// rewriting it would only risk turning a good file into a torn one.
	if fi, err := os.Stat(p); err != nil || fi.Size() != int64(len(body)) {
		if err := store.WriteFileAtomic(p, body); err != nil {
			return "", fmt.Errorf("cache put %s: %w", key, err)
		}
	}

	e := core.ManifestEntry{
		URL:       url,
		FetchedAt: time.Now().UTC(),
		Bytes:     len(body),
		SHA256:    sha,
		Status:    "ok",
	}
	if resp != nil {
		e.HTTPStatus = resp.StatusCode
		e.ETag = resp.Header.Get("ETag")
		e.LastModified = resp.Header.Get("Last-Modified")
	}

	c.mu.Lock()
	// Kind and Name are the caller's to set; deriving a kind from a path would be
	// a guess, so carry forward what is already recorded.
	if old, ok := c.index.Entries[key]; ok {
		e.Kind, e.Name = old.Kind, old.Name
	}
	c.index.Entries[key] = e
	// This run fetched it, so this run's entry is the newest thing anyone knows
	// about the key and wins the merge in Save.
	c.dirty[key] = true
	c.mu.Unlock()
	if c.Persist {
		if err := c.Save(core.NewManifest()); err != nil {
			return sha, fmt.Errorf("cache put %s: saving the index: %w", key, err)
		}
	}
	return sha, nil
}

// Get returns the cached bytes and what is known about the transfer. The bool
// reports whether the BYTES are there: a blobless entry still has usable ETags.
func (c *Cache) Get(key string) ([]byte, core.ManifestEntry, bool) {
	if c.NoServe {
		return nil, core.ManifestEntry{}, false
	}
	c.mu.Lock()
	e, ok := c.index.Entries[strings.TrimSpace(key)]
	c.mu.Unlock()
	if !ok {
		return nil, core.ManifestEntry{}, false
	}
	b, ok := c.GetBySHA(e.SHA256)
	if !ok {
		return nil, e, false
	}
	return b, e, true
}

// FreshEnough reports whether a body fetched at t may still answer: maxAge 0 means
// def, a negative one never expires, and a zero t (no FetchedAt) has expired.
func FreshEnough(t time.Time, now func() time.Time, maxAge, def time.Duration) bool {
	if maxAge == 0 {
		maxAge = def
	}
	if maxAge < 0 {
		return true
	}
	return !t.IsZero() && now().Sub(t) < maxAge
}

// GetBySHA returns a blob by its address, verifying it still hashes to it. The
// shape check is also the path guard: index.json is a file on disk, not trusted input.
func (c *Cache) GetBySHA(sha string) ([]byte, bool) {
	if !isSHA256Hex(sha) {
		return nil, false
	}
	b, err := os.ReadFile(c.blobPath(sha))
	if err != nil || len(b) == 0 {
		return nil, false
	}
	if store.SHA256Hex(b) != sha {
		return nil, false
	}
	return b, true
}

// Load returns the index as a copy: the caller holds it for the length of a run
// while the workers are still calling Put.
func (c *Cache) Load() core.Manifest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return core.Manifest{Tool: c.index.Tool, Entries: maps.Clone(c.index.Entries)}
}

// Save merges m into the index on DISK, under a lockfile: a key in m wins, a
// key only Put recorded survives, and two openers cannot lose each other's entries.
func (c *Cache) Save(m core.Manifest) error {
	if err := os.MkdirAll(c.Dir(), 0o755); err != nil {
		return err
	}
	unlock, err := c.lockIndex()
	if err != nil {
		return err
	}
	defer unlock()

	c.mu.Lock()
	for k, e := range m.Entries {
		// The caller's half of an entry is news whenever it differs from the index.
		// Save(Load()) asserts nothing new and must not mark the whole index dirty.
		if old, ok := c.index.Entries[k]; !ok || old != e {
			c.dirty[k] = true
		}
	}
	maps.Copy(c.index.Entries, m.Entries)
	out := core.NewManifest()
	// Everything the index on disk holds NOW; with no readable file the base is
	// what THIS process holds, or a run would orphan every blob it did not fetch.
	if onDisk, ok := readIndexFile(c.Root); ok {
		maps.Copy(out.Entries, onDisk.Entries)
	} else {
		maps.Copy(out.Entries, c.index.Entries)
	}
	for k := range c.dirty {
		if e, ok := c.index.Entries[k]; ok {
			out.Entries[k] = e
		}
	}
	c.mu.Unlock()

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(filepath.Join(c.Dir(), cacheIndexFile), append(b, '\n'))
}

// indexLockName is the index lockfile, an O_EXCL create in the cache directory .gitignore
// excludes; the loop reading the two durations below is store.LockPath.
const indexLockName = "index.lock"

// var rather than const so the lock tests can shorten them. Save's critical
// section is a read, a merge and a rename, so a two-minute lockfile is dead.
var (
	indexLockWait  = 5 * time.Second
	indexLockStale = 2 * time.Minute
)

// lockIndex serialises Save against every other writer of index.json. The
// durations are read here, so the tests' shortened values take effect.
func (c *Cache) lockIndex() (func(), error) {
	unlock, err := store.LockPath(filepath.Join(c.Dir(), indexLockName), indexLockWait, indexLockStale)
	if err != nil {
		return nil, fmt.Errorf("cache index: %w", err)
	}
	return unlock, nil
}

// isSHA256Hex reports whether s is exactly 64 lower-case hex digits, which is
// what store.SHA256Hex produces and the only thing blobPath may be handed.
func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// CacheDirName and CacheIndexFile are where a cache lives, relative to the
// repository root, exported for the cold-start acceptance in internal/etf/app.
const (
	CacheDirName   = cacheDir
	CacheIndexFile = cacheIndexFile
)
