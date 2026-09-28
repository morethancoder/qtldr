package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
