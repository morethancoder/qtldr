package gitx

import (
	"testing"
)

func TestParseChurn(t *testing.T) {
	out := "\x00commit\n\na/x.go\na/y.go\n\x00commit\n\na/x.go\nmain.go\n\x00commit\n\nb/z.go\n"
	c := ParseChurn(out)
	if c.Files["a/x.go"] != 2 || c.Files["a/y.go"] != 1 || c.Files["main.go"] != 1 {
		t.Errorf("files %v", c.Files)
	}
	if c.Dirs["a"] != 2 || c.Dirs["."] != 1 || c.Dirs["b"] != 1 {
		t.Errorf("dirs %v", c.Dirs)
	}
}

// Hand-written diff text (pure parser test).
const diff = `diff --git a/internal/pricing/tier.go b/internal/pricing/tier.go
index 1111111..2222222 100644
--- a/internal/pricing/tier.go
+++ b/internal/pricing/tier.go
@@ -29 +29 @@ func applyTiered(tiers []Tier, qty int, unit money.Amount) (money.Amount, error) {
-		if qty >= t.Floor && t.Floor > best.Floor {
+		if qty > t.Floor && t.Floor > best.Floor {
@@ -40,2 +40,0 @@ func applyTiered(
-	x
-	y
@@ -50,0 +49,3 @@
+a
+b
+c
diff --git a/gone.go b/gone.go
deleted file mode 100644
--- a/gone.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package x
`

func TestParseDiff(t *testing.T) {
	h := ParseDiff(diff)
	got := h["internal/pricing/tier.go"]
	want := []Range{{29, 29, false}, {40, 40, true}, {49, 51, false}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(h) != 1 {
		t.Errorf("deleted files have no new lines: %v", h)
	}
}

func TestTouches(t *testing.T) {
	cases := []struct {
		r        Range
		from, to int
		want     bool
	}{
		{Range{29, 29, false}, 20, 48, true},
		{Range{10, 19, false}, 20, 48, false},
		{Range{15, 20, false}, 20, 48, true},
		{Range{48, 60, false}, 20, 48, true},
		{Range{40, 40, true}, 20, 48, true},
		{Range{48, 48, true}, 20, 48, false}, // deleted just after the closing brace
		{Range{19, 19, true}, 20, 48, false}, // deleted just before the func line
	}
	for _, c := range cases {
		if got := c.r.Touches(c.from, c.to); got != c.want {
			t.Errorf("%+v touches [%d,%d] = %v, want %v", c.r, c.from, c.to, got, c.want)
		}
	}
}

func TestFunctionChurn(t *testing.T) {
	log := "\x00commit\n\ndiff --git a/p/a.go b/p/a.go\n--- a/p/a.go\n+++ b/p/a.go\n@@ -3 +3 @@\n-x\n+y\n@@ -20,0 +21,2 @@\n+a\n+b\n" +
		"\x00commit\n\ndiff --git a/p/a.go b/p/a.go\n--- /dev/null\n+++ b/p/a.go\n@@ -0,0 +1,30 @@\n+new file\n"
	commits := ParseLogPatch(log)
	if len(commits) != 2 {
		t.Fatalf("commits %v", commits)
	}
	got := CountFunctionChurn(commits, []Span{{ID: "f", File: "p/a.go", From: 1, To: 5}, {ID: "g", File: "p/a.go", From: 21, To: 25}, {ID: "h", File: "p/b.go", From: 1, To: 9}})
	if got["f"] != 2 || got["g"] != 2 || got["h"] != 0 {
		t.Fatalf("got %v", got)
	}
}
