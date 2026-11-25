package cache

// helpers_test.go - the shared helpers the old cache_test.go defined, kept for
// noserve_test.go and save_test.go when that file went with the snapshot store.

import (
	"net/http"
	"testing"
)

func openCache(t *testing.T) *Cache {
	t.Helper()
	c, err := OpenCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// resp builds the half of an http.Response the cache reads: the status and the
// two validators a conditional GET sends back.
func resp(status int, etag, lastMod string) *http.Response {
	r := &http.Response{StatusCode: status, Header: http.Header{}}
	if etag != "" {
		r.Header.Set("ETag", etag)
	}
	if lastMod != "" {
		r.Header.Set("Last-Modified", lastMod)
	}
	return r
}
