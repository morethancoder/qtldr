package focus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
)

func pf(v float64) *float64 { return &v }

func TestPrompt(t *testing.T) {
	th := config.Default().Thresholds
	fn := model.Detail{
		Node: model.Node{ID: "github.com/acme/ledger/internal/pricing.applyTiered", Kind: model.KindFunc, File: "internal/pricing/tier.go", Line: 20},
		Metrics: &model.Metrics{CRAP: pf(14.08), CC: model.Ptr(11), Cognitive: model.Ptr(11),
			Coverage: &model.Coverage{Percent: pf(70.6), Lines: &model.LineStates{Uncovered: []int{25, 26, 37}}},
			Mutation: &model.Mutation{Killed: 9, Survived: 1, NotCovered: 3, Score: pf(90), Mutants: []model.Mutant{{Line: 29, Status: "LIVED", Description: ">= → >"}}}},
	}
	got := Prompt(fn, th, []notes.Placed{{Note: notes.Note{Author: "user", Text: "Split by Kind."}, Line: 34}}, " keep the API ")
	for _, want := range []string{
		"Improve pricing.applyTiered (internal/pricing/tier.go:20). CRAP 14.1 (target 8 or less), CC 11, cognitive 11, coverage 70.6% (target 80%+), mutation 90% (target 70%+), 1 surviving and 3 not-covered mutants.",
		"Surviving mutants: tier.go:29 >= → >.",
		"Not covered: tier.go:25–26, 37.",
		"Note on line 34 (user): Split by Kind.",
		"From the user: keep the API",
		"Verify with: qtldr check pricing.applyTiered",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	pkg := model.Detail{Node: model.Node{ID: "m/internal/pricing", Kind: model.KindPackage, Name: "internal/pricing"},
		Metrics: &model.Metrics{CrapMax: pf(30), Worst: "m/internal/pricing.BestRule", Coverage: &model.Coverage{Percent: pf(70)}}}
	if got := Prompt(pkg, th, nil, ""); !strings.Contains(got, "Improve package internal/pricing: CRAP max 30.0 (pricing.BestRule), coverage 70% (target 80%+), mutation not measured.") ||
		!strings.HasSuffix(got, "Verify with: qtldr check internal/pricing") {
		t.Errorf("package prompt:\n%s", got)
	}
	ty := model.Detail{Node: model.Node{ID: "m/p.Rule", Kind: model.KindType, File: "p/types.go", Line: 11}}
	if got := Prompt(ty, th, nil, ""); !strings.HasPrefix(got, "Review type p.Rule (p/types.go:11) and its usages.") {
		t.Errorf("type prompt: %s", got)
	}
	ext := model.Detail{Node: model.Node{ID: "github.com/go-chi/chi/v5", Kind: model.KindExternal, Name: "go-chi/chi"}}
	if got := Prompt(ext, th, nil, ""); got != "List every place the code imports go-chi/chi." {
		t.Errorf("external prompt: %s", got)
	}
	bare := model.Detail{Node: model.Node{ID: "m/p.f", Kind: model.KindFunc}}
	if got := Prompt(bare, th, nil, ""); !strings.Contains(got, "CRAP not measured, CC —, cognitive —, coverage not measured, mutation not measured") {
		t.Errorf("nothing measured: %s", got)
	}
}

func TestAgentPing(t *testing.T) {
	root := t.TempDir()
	if _, err := ReadAgentPing(root); err == nil {
		t.Error("no ping yet: want error")
	}
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if err := WriteAgentPing(root, "claude-code", at); err != nil {
		t.Fatal(err)
	}
	if p, err := ReadAgentPing(root); err != nil || p.Client != "claude-code" || !p.At.Equal(at) {
		t.Errorf("ping: %+v %v", p, err)
	}
}

func TestSendTmuxWithoutTmux(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no tmux here
	err := SendTmux(context.Background(), "agent:1", "fix it")
	if err == nil || !strings.HasPrefix(err.Error(), "tmux send-keys -t agent:1 failed: ") || !strings.HasSuffix(err.Error(), "check [agent].tmux_target") {
		t.Errorf("error: %v", err)
	}
}

func TestReadWrite(t *testing.T) {
	root := t.TempDir()
	if _, err := Read(root); !errors.Is(err, ErrNone) {
		t.Fatalf("err = %v", err)
	}
	f := Focus{ID: "m/p.f", Level: "function", SelectedLine: 29, Sent: true, At: time.Date(2026, 9, 28, 10, 4, 0, 0, time.UTC)}
	if err := Write(root, f); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root)
	if err != nil || got != f {
		t.Fatalf("%+v %v", got, err)
	}
}
