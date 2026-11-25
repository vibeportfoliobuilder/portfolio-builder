package cache

// noserve_test.go - NoServe is "do not read from the cache" as a property of the
// cache itself. Two halves: never serving, and storing anyway.

import (
	"os"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
)

// TestNoServeStoresEverythingAndServesNothing pins both halves: the second is
// what makes the flag worth having, since the next run reuses what it stored.
func TestNoServeStoresEverythingAndServesNothing(t *testing.T) {
	const key = "funds/IE0000000111/nav.csv"
	body := []byte("date,nav\n2026-08-06,10\n")

	root := t.TempDir()
	c, err := OpenCache(root)
	if err != nil {
		t.Fatal(err)
	}
	c.NoServe = true

	sha, err := c.Put(key, "https://n.test/n", body, resp(200, "\"etag\"", ""))
	if err != nil {
		t.Fatal(err)
	}

	// Never serves the bytes, and not the entry either: a 304 off its validators
	// would leave this run with neither copy.
	b, e, ok := c.Get(key)
	if ok {
		t.Error("Get reported a hit on a cache that is not allowed to serve one")
	}
	if b != nil {
		t.Errorf("Get returned %d bytes on a miss", len(b))
	}
	if e != (core.ManifestEntry{}) {
		t.Errorf("Get returned the entry %+v, whose ETag would turn the re-download into a 304", e)
	}

	// Stores anyway: the blob is on disk under its own address...
	if got, err := os.ReadFile(c.blobPath(sha)); err != nil || string(got) != string(body) {
		t.Errorf("the blob was not stored: %q %v", got, err)
	}
	// ...and the index names it, which is what makes the blob reachable at all.
	if err := c.Save(core.NewManifest()); err != nil {
		t.Fatal(err)
	}
	serving, err := OpenCache(root)
	if err != nil {
		t.Fatal(err)
	}
	got, entry, ok := serving.Get(key)
	if !ok {
		t.Fatal("the entry a NoServe run stored is not in the index it wrote; its blob is orphaned")
	}
	if string(got) != string(body) {
		t.Errorf("the stored bytes are %q, want %q", got, body)
	}
	if entry.SHA256 != sha {
		t.Errorf("the entry addresses %q, want the sha Put returned, %q", entry.SHA256, sha)
	}
	if entry.ETag != "\"etag\"" {
		t.Errorf("the entry carries ETag %q; the transfer bookkeeping is recorded whether or not this run may read it", entry.ETag)
	}
}

// TestNoServeIsOffUnlessItIsAskedFor: the zero value has to be the serving
// cache, since every existing caller builds one with OpenCache.
func TestNoServeIsOffUnlessItIsAskedFor(t *testing.T) {
	c := openCache(t)
	if c.NoServe {
		t.Fatal("a cache opened the ordinary way refuses to serve")
	}
	const key = "funds/IE0000000112/nav.csv"
	if _, err := c.Put(key, "https://n.test/n", []byte("x"), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.Get(key); !ok {
		t.Error("an ordinary cache missed on a key it had just stored")
	}
}
