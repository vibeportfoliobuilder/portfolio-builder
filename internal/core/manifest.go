package core

// manifest.go - the provenance index the raw cache keeps beside its blobs.

import "time"

// ArtifactKind names what a cached body is; the cache keys alias entries by it.
type ArtifactKind string

// ManifestEntry is one cached body: where it came from, when, and its hash.
type ManifestEntry struct {
	Kind      ArtifactKind `json:"kind"`
	Name      string       `json:"name,omitempty"`
	URL       string       `json:"url,omitempty"`
	FetchedAt time.Time    `json:"fetched_at"`
	Bytes     int          `json:"bytes"`
	SHA256    string       `json:"sha256"`
	// Status describes the CONTENT held in the snapshot, not the transfer: ok,
	// failed, pending, manual, distilled, unconverted or unfetchable.
	Status       string `json:"status"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	Note         string `json:"note,omitempty"`
	// Canonical records what was stripped or converted, empty meaning verbatim.
	// The sha256 above is always of what is on disk.
	Canonical string `json:"canonical,omitempty"`
	// Sheet is the worksheet a converted table came from - for VanEck and Xtrackers
	// the only place the holdings as-of date is stated. Empty when there is none.
	Sheet string `json:"sheet,omitempty"`
	// Path names the file that holds the bytes when it is not the artefact's own
	// path. Empty is the normal case and means the two are the same.
	Path string `json:"path,omitempty"`
	// Universe fingerprints the ISINs a distilled copy was cut down to, empty for
	// a file that is stored whole. A publisher's validators do not cover this half.
	Universe string `json:"universe,omitempty"`
}

// Fetched reports that this entry's download landed, readable as csv ("ok") or
// held verbatim ("unconverted"); failed and pending did not.
func (e ManifestEntry) Fetched() bool { return e.Status == "ok" || e.Status == "unconverted" }

// Held reports that the snapshot holds bytes for this entry: fetched, or placed
// by hand ("manual").
func (e ManifestEntry) Held() bool { return e.Fetched() || e.Status == "manual" }

// Manifest is the cache index, keyed by cache key.
type Manifest struct {
	Tool    string                   `json:"tool"`
	Entries map[string]ManifestEntry `json:"entries"` // keyed by repo-relative path
}

// NewManifest returns an empty manifest carrying the tool name that every
// snapshot is stamped with.
func NewManifest() Manifest {
	return Manifest{Tool: "etfcompare", Entries: map[string]ManifestEntry{}}
}
