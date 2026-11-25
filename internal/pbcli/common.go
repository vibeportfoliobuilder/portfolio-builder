package pbcli

// common.go - the raw cache outside the repository, and lookups shared by commands.

import (
	"os"
	"path/filepath"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/cache"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/core"
)

// rawCacheDir is os.UserCacheDir()/pb/raw: raw responses never land in the repository.
func rawCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "pb", "raw"), nil
}

// rawCache opens the shared raw cache for the screener, Tavily, fetch-url and any connector.
func rawCache() (*cache.Cache, error) {
	d, err := rawCacheDir()
	if err != nil {
		return nil, err
	}
	c, err := cache.OpenCache(d)
	if err != nil {
		return nil, err
	}
	c.Persist = true
	return c, nil
}

// countryCode is a country name as an alpha-2 code, or the input upper-cased.
func countryCode(name string) string {
	if c := core.CountryCode(name); c != "" {
		return c
	}
	return name
}
