package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/model"
)

func TestSnapshotRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, err := ReadSnapshot(root); !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("empty dir: err = %v, want ErrNoSnapshot", err)
	}
	s := model.Snapshot{Schema: model.SchemaVersion, Module: "m", Generated: time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)}
	s.Nodes = []model.Node{{ID: "m", Kind: model.KindModule, Name: "m"}}
	if err := WriteSnapshot(root, s); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSnapshot(root)
	if err != nil || got.Module != "m" || len(got.Nodes) != 1 || !got.Generated.Equal(s.Generated) {
		t.Fatalf("got %+v %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, Dir))
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestReadSnapshotRejectsOtherSchema(t *testing.T) {
	root := t.TempDir()
	if err := WriteJSON(SnapshotPath(root), map[string]int{"schema": 99}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSnapshot(root); err == nil {
		t.Fatal("want a schema error")
	}
}

func TestCoverageCache(t *testing.T) {
	root := t.TempDir()
	if c, err := ReadCoverageCache(root); err != nil || c.Schema != coverage.CacheSchema || c.PackageErrors == nil {
		t.Fatalf("missing file: %+v %v", c, err)
	}
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	want := coverage.NewCache()
	want.RanAt = &at
	want.PackageErrors["m/p"] = "tests failed"
	if err := WriteCoverageCache(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCoverageCache(root)
	if err != nil || got.RanAt == nil || !got.RanAt.Equal(at) || got.PackageErrors["m/p"] != "tests failed" {
		t.Errorf("round trip: %+v %v", got, err)
	}

	noErrors := coverage.NewCache()
	noErrors.PackageErrors = nil
	if err := WriteCoverageCache(root, noErrors); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadCoverageCache(root); got.PackageErrors == nil {
		t.Error("package errors must never be nil")
	}

	for _, body := range []string{"not json", `{"schema": 99, "ran_at": "2026-09-29T12:00:00Z"}`} {
		if err := os.WriteFile(CachePath(root, "coverage.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, err := ReadCoverageCache(root); err != nil || got.RanAt != nil || got.Schema != coverage.CacheSchema {
			t.Errorf("%s: want an empty cache, got %+v %v", body, got, err)
		}
	}

	dirRoot := t.TempDir()
	if err := os.MkdirAll(CachePath(dirRoot, "coverage.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCoverageCache(dirRoot); err == nil {
		t.Error("coverage.json is a directory: want error")
	}
}

func TestWriteLog(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 9, 29, 12, 0, 5, 0, time.FixedZone("x", 3600))
	rel, err := WriteLog(root, "coverage", at, []byte("output"))
	if err != nil || rel != ".qtldr/logs/coverage-20260929-110005.log" {
		t.Fatalf("log path %q %v", rel, err)
	}
	if b, err := os.ReadFile(filepath.Join(root, rel)); err != nil || string(b) != "output" {
		t.Errorf("log content %q %v", b, err)
	}
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, Dir), []byte("a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rel, err := WriteLog(blocked, "coverage", at, nil); err == nil || rel != "" {
		t.Errorf("unwritable: %q %v", rel, err)
	}
}

func TestWriteFileErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(file, "sub", "x.json"), []byte("{}")); err == nil {
		t.Error("parent is a file: want error")
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(ro, "x.json"), []byte("{}")); err == nil {
		t.Error("read-only dir: want error")
	}
	if err := WriteJSON(filepath.Join(dir, "bad.json"), func() {}); err == nil {
		t.Error("unencodable value: want error")
	}
}
