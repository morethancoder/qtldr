package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/store"
)

func TestBuiltin(t *testing.T) {
	ts, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 9 || ts[0].ID != "gruvbox-dark" || ts[0].Shiki != "gruvbox-dark-hard" {
		t.Fatalf("got %d themes, first %+v", len(ts), ts[0].ID)
	}
}

func TestLoadCustom(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, store.Dir, "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := Builtin()
	good := b[0]
	good.ID, good.Name = "my-theme", "Mine"
	write := func(name string, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a-good.json", `{"id":"my-theme","name":"Mine","dark":true,"ui":`+jsonMap(good.UI)+`,"grade":`+jsonMap(good.Grade)+`,"syntax":`+jsonMap(good.Syntax)+`}`)
	write("b-dup.json", `{"id":"nord","name":"N","ui":`+jsonMap(good.UI)+`,"grade":`+jsonMap(good.Grade)+`,"syntax":`+jsonMap(good.Syntax)+`}`)
	write("c-bad.json", `{"id":"x","name":"X","ui":{"bg":"red"}}`)
	write("d-broken.json", `{`)
	all, warnings, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 10 || !all[9].Custom || all[9].ID != "my-theme" || !Find(all, "my-theme") {
		t.Fatalf("got %d themes", len(all))
	}
	if len(warnings) != 3 || !strings.Contains(warnings[0], "built-in") || !strings.Contains(warnings[1], "ui.bg must be a #rrggbb color") {
		t.Fatalf("warnings %v", warnings)
	}
}

func jsonMap(m map[string]string) string {
	parts := []string{}
	for k, v := range m {
		parts = append(parts, `"`+k+`":"`+v+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
