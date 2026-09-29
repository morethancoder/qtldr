package web

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

func TestDist(t *testing.T) {
	b, err := fs.ReadFile(Dist(), "index.html")
	if err != nil || !strings.Contains(string(b), "<div id=\"root\">") {
		t.Errorf("index.html: %v\n%s", err, b)
	}
}

func TestThemes(t *testing.T) {
	var themes any
	if err := json.Unmarshal(Themes, &themes); err != nil {
		t.Errorf("themes.json: %v", err)
	}
}
