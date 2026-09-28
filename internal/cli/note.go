package cli

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
)

func noteFlags(fs *flag.FlagSet) {
	fs.Int("line", 0, "with add: the source line the note is about")
	fs.Bool("all", false, "with list: include resolved notes")
}

func runNote(e *env, fs *flag.FlagSet, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: note add <id> [--line N] <text> | note list [id] | note resolve <note-id>", errUsage)
	}
	root, err := e.moduleRoot()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		return noteAdd(e, root, fs, args[1:])
	case "list":
		return noteList(e, root, boolFlag(fs, "all"), args[1:])
	case "resolve":
		return noteResolve(e, root, args[1:])
	}
	return fmt.Errorf("%w: unknown note command %q; use add, list or resolve", errUsage, args[0])
}

func noteAdd(e *env, root string, fs *flag.FlagSet, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("%w: note add <id> [--line N] <text>", errUsage)
	}
	snap, err := e.readSnapshot()
	if err != nil {
		return err
	}
	line := fs.Lookup("line").Value.(flag.Getter).Get().(int)
	n, err := notes.Add(root, snap, args[0], line, strings.Join(args[1:], " "), "user", time.Now().UTC().Truncate(time.Second))
	if err != nil {
		return err
	}
	if e.g.json {
		return e.printJSON(n)
	}
	fmt.Fprintf(e.stdout, "Added %s on %s.\n", n.ID, check.Short(n.Target))
	return nil
}

func noteList(e *env, root string, all bool, args []string) error {
	list, err := notes.Load(root)
	if err != nil {
		return err
	}
	snap, snapErr := e.readSnapshot()
	target, err := noteTarget(snap, snapErr, args)
	if err != nil {
		return err
	}
	shown := filterNotes(list, target, all)
	if e.g.json {
		return e.printJSON(shown)
	}
	printNotes(e, root, snap, shown)
	return nil
}

// noteTarget resolves the optional ID argument of `note list`.
func noteTarget(snap model.Snapshot, snapErr error, args []string) (model.ID, error) {
	if len(args) == 0 {
		return "", nil
	}
	if snapErr != nil {
		return "", snapErr
	}
	return model.Resolve(snap.IDs(), args[0])
}

func printNotes(e *env, root string, snap model.Snapshot, shown []notes.Note) {
	if len(shown) == 0 {
		fmt.Fprintln(e.stdout, "No notes.")
	}
	for _, n := range shown {
		fmt.Fprintln(e.stdout, noteLine(root, snap, n))
	}
}

func filterNotes(list []notes.Note, target model.ID, all bool) []notes.Note {
	out := []notes.Note{}
	for _, n := range list {
		if (target == "" || n.Target == target) && (all || !n.Resolved) {
			out = append(out, n)
		}
	}
	return out
}

// noteLine is "n_… pricing.applyTiered:34  user · 2026-09-26  text".
func noteLine(root string, snap model.Snapshot, n notes.Note) string {
	where := check.Short(n.Target)
	if node, ok := snap.Node(n.Target); ok && n.LineOffset != nil {
		p := notes.Place(n, node.Line, notes.FuncLines(root, node))
		where = fmt.Sprintf("%s:%d", where, p.Line)
		if p.Outdated {
			where += " (code changed)"
		}
	}
	state := ""
	if n.Resolved {
		state = " [resolved]"
	}
	return fmt.Sprintf("%s  %s  %s · %s%s\n    %s", n.ID, where, n.Author, n.Created.Format("2006-01-02"), state, n.Text)
}

func noteResolve(e *env, root string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%w: note resolve <note-id>", errUsage)
	}
	n, err := notes.Update(root, args[0], func(n *notes.Note) { n.Resolved = true })
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "Resolved %s.\n", n.ID)
	return nil
}
