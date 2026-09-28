// Package store reads and writes the files under .qtldr/ atomically.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/morethancoder/qtldr/internal/model"
)

// Dir is the state directory at the module root.
const Dir = ".qtldr"

// SnapshotPath returns the path of snapshot.json under root.
func SnapshotPath(root string) string { return filepath.Join(root, Dir, "snapshot.json") }

// ErrNoSnapshot means analyze has not run yet.
var ErrNoSnapshot = errors.New("no snapshot yet")

// WriteSnapshot writes the snapshot as indented JSON.
func WriteSnapshot(root string, s model.Snapshot) error {
	return WriteJSON(SnapshotPath(root), s)
}

// ReadSnapshot reads the snapshot; the error wraps ErrNoSnapshot if there is none.
func ReadSnapshot(root string) (model.Snapshot, error) {
	var s model.Snapshot
	p := SnapshotPath(root)
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return s, fmt.Errorf("%w at %s; run `qtldr analyze` first", ErrNoSnapshot, p)
	}
	if err != nil {
		return s, fmt.Errorf("read %s: %w", p, err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("%s is not a valid snapshot (%v); run `qtldr analyze` to rebuild it", p, err)
	}
	if s.Schema != model.SchemaVersion {
		return s, fmt.Errorf("%s has schema %d, this qtldr reads %d; run `qtldr analyze` to rebuild it", p, s.Schema, model.SchemaVersion)
	}
	return s, nil
}

// WriteJSON writes v as indented JSON to path via a temporary file and a
// rename, so readers never see a partial file.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return WriteFile(path, append(b, '\n'))
}

// WriteFile writes b to path atomically, creating parent directories.
func WriteFile(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
