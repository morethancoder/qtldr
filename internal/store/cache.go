package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/morethancoder/qtldr/internal/coverage"
)

// CachePath returns a path under .qtldr/cache/.
func CachePath(root, name string) string { return filepath.Join(root, Dir, "cache", name) }

// ReadCoverageCache reads .qtldr/cache/coverage.json; a missing or
// unreadable-schema file yields an empty cache.
func ReadCoverageCache(root string) (coverage.Cache, error) {
	c := coverage.NewCache()
	p := CachePath(root, "coverage.json")
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read %s: %w", p, err)
	}
	var read coverage.Cache
	if err := json.Unmarshal(b, &read); err != nil || read.Schema != coverage.CacheSchema {
		return c, nil // rebuilt by the next coverage run
	}
	if read.PackageErrors == nil {
		read.PackageErrors = c.PackageErrors
	}
	return read, nil
}

// WriteCoverageCache writes .qtldr/cache/coverage.json.
func WriteCoverageCache(root string, c coverage.Cache) error {
	return WriteJSON(CachePath(root, "coverage.json"), c)
}

// WriteLog writes .qtldr/logs/<kind>-<time>.log and returns its path relative
// to root.
func WriteLog(root, kind string, at time.Time, b []byte) (string, error) {
	rel := filepath.Join(Dir, "logs", kind+"-"+at.UTC().Format("20060102-150405")+".log")
	if err := WriteFile(filepath.Join(root, rel), b); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
