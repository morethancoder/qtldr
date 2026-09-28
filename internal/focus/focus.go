// Package focus records what the human is pointing at (.qtldr/focus.json)
// and builds the self-contained "fix prompt" for an agent (PLAN.md §9.2).
package focus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
	"github.com/morethancoder/qtldr/internal/store"
)

// Focus is the content of .qtldr/focus.json.
type Focus struct {
	ID           model.ID  `json:"id"`
	Level        string    `json:"level"` // module | package | function
	SelectedLine int       `json:"selected_line,omitempty"`
	Message      string    `json:"message,omitempty"`
	Sent         bool      `json:"sent"`
	At           time.Time `json:"at"`
}

// Path is .qtldr/focus.json under root.
func Path(root string) string { return filepath.Join(root, store.Dir, "focus.json") }

// Write saves the focus.
func Write(root string, f Focus) error { return store.WriteJSON(Path(root), f) }

// ErrNone means nothing has been selected yet.
var ErrNone = errors.New("nothing is selected in the qtldr UI yet; run `qtldr serve` and click a box")

// Read loads the focus.
func Read(root string) (Focus, error) {
	var f Focus
	b, err := os.ReadFile(Path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return f, ErrNone
	}
	if err != nil {
		return f, err
	}
	return f, json.Unmarshal(b, &f)
}

// Prompt builds a self-contained fix prompt for the node in d: ID, file and
// line, scores against targets, survivors, uncovered ranges, notes, and how
// to verify.
func Prompt(d model.Detail, th config.Thresholds, placed []notes.Placed, message string) string {
	var b strings.Builder
	name := path.Base(string(d.Node.ID))
	switch d.Node.Kind {
	case model.KindFunc:
		functionPrompt(&b, d, th, name)
	case model.KindPackage:
		packagePrompt(&b, d, th, d.Node.Name)
		name = d.Node.Name
	case model.KindType:
		fmt.Fprintf(&b, "Review type %s (%s:%d) and its usages.", name, d.Node.File, d.Node.Line)
	default:
		fmt.Fprintf(&b, "List every place the code imports %s.", d.Node.Name)
		return b.String()
	}
	for _, p := range placed {
		fmt.Fprintf(&b, "\nNote%s (%s): %s", lineRef(p), p.Author, p.Text)
	}
	if message = strings.TrimSpace(message); message != "" {
		fmt.Fprintf(&b, "\nFrom the user: %s", message)
	}
	fmt.Fprintf(&b, "\nVerify with: qtldr check %s", name)
	return b.String()
}

func lineRef(p notes.Placed) string {
	if p.Line == 0 {
		return ""
	}
	return fmt.Sprintf(" on line %d", p.Line)
}

func functionPrompt(b *strings.Builder, d model.Detail, th config.Thresholds, name string) {
	m := metricsOf(d)
	fmt.Fprintf(b, "Improve %s (%s:%d). %s, CC %s, cognitive %s, %s, %s.", name, d.Node.File, d.Node.Line,
		crapText(m.CRAP, th.CrapMax), intText(m.CC), intText(m.Cognitive), coverageText(m.Coverage, th.CoverageMin), mutationText(m.Mutation, th.MutationMin))
	file := path.Base(d.Node.File)
	if s := survivors(m.Mutation, file); s != "" {
		fmt.Fprintf(b, "\nSurviving mutants: %s.", s)
	}
	if c := m.Coverage; c != nil && c.Lines != nil && len(c.Lines.Uncovered) > 0 {
		fmt.Fprintf(b, "\nNot covered: %s:%s.", file, strings.Join(coverage.Ranges(c.Lines.Uncovered), ", "))
	}
	b.WriteString("\nKill the survivors with focused tests first, then reduce complexity.")
}

func packagePrompt(b *strings.Builder, d model.Detail, th config.Thresholds, name string) {
	m := metricsOf(d)
	worst := ""
	if m.Worst != "" {
		worst = " (" + path.Base(string(m.Worst)) + ")"
	}
	fmt.Fprintf(b, "Improve package %s: CRAP max %s%s, %s, %s.", name, floatText(m.CrapMax), worst,
		coverageText(m.Coverage, th.CoverageMin), mutationText(m.Mutation, th.MutationMin))
	b.WriteString("\nStart with the riskiest function, kill surviving mutants with focused tests, then reduce complexity.")
}

func metricsOf(d model.Detail) model.Metrics {
	if d.Metrics == nil {
		return model.Metrics{}
	}
	return *d.Metrics
}

func crapText(v *float64, limit float64) string {
	if v == nil {
		return "CRAP not measured"
	}
	return fmt.Sprintf("CRAP %.1f (target %g or less)", *v, limit)
}

func coverageText(c *model.Coverage, limit float64) string {
	switch {
	case c == nil:
		return "coverage not measured"
	case c.Percent == nil:
		return "no statements"
	}
	s := fmt.Sprintf("coverage %g%% (target %g%%+)", *c.Percent, limit)
	if c.Stale {
		s += ", stale"
	}
	return s
}

func mutationText(mu *model.Mutation, limit float64) string {
	switch {
	case mu == nil:
		return "mutation not measured"
	case mu.Score == nil:
		return fmt.Sprintf("mutation: no mutant ran (%d not covered)", mu.NotCovered)
	}
	return fmt.Sprintf("mutation %g%% (target %g%%+), %d surviving and %d not-covered mutants", *mu.Score, limit, mu.Survived, mu.NotCovered)
}

func survivors(mu *model.Mutation, file string) string {
	if mu == nil {
		return ""
	}
	var parts []string
	for _, m := range mu.Mutants {
		if m.Status == "LIVED" {
			parts = append(parts, fmt.Sprintf("%s:%d %s", file, m.Line, m.Description))
		}
	}
	return strings.Join(parts, "; ")
}

func floatText(v *float64) string {
	if v == nil {
		return "not measured"
	}
	return fmt.Sprintf("%.1f", *v)
}

func intText(v *int) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprint(*v)
}

// SendTmux types prompt into a tmux pane and presses Enter.
func SendTmux(ctx context.Context, target, prompt string) error {
	if out, err := exec.CommandContext(ctx, "tmux", "send-keys", "-t", target, "-l", prompt).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys -t %s failed: %v %s; check [agent].tmux_target", target, err, strings.TrimSpace(string(out)))
	}
	return exec.CommandContext(ctx, "tmux", "send-keys", "-t", target, "Enter").Run()
}

// AgentPing is written by `qtldr mcp` on every tool call, so the UI can show
// that an agent is connected.
type AgentPing struct {
	Client string    `json:"client"`
	At     time.Time `json:"at"`
}

func agentPath(root string) string { return filepath.Join(root, store.Dir, "cache", "agent.json") }

// WriteAgentPing records that client used qtldr at t.
func WriteAgentPing(root, client string, t time.Time) error {
	return store.WriteJSON(agentPath(root), AgentPing{Client: client, At: t})
}

// ReadAgentPing returns the last ping.
func ReadAgentPing(root string) (AgentPing, error) {
	var p AgentPing
	b, err := os.ReadFile(agentPath(root))
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal(b, &p)
}
