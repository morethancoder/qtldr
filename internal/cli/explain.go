package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/glossary"
)

func runExplain(e *env, _ *flag.FlagSet, args []string) error {
	cfg := config.Default()
	if root, err := e.moduleRoot(); err == nil {
		if c, err := e.loadConfig(root); err == nil {
			cfg = c
		}
	}
	g := glossary.Load(cfg)
	if len(args) == 0 {
		if e.g.json {
			return e.printJSON(g)
		}
		fmt.Fprintf(e.stdout, "Terms: %s\nRun: qtldr explain <term>\n", strings.Join(g.Keys(), ", "))
		return nil
	}
	t, err := g.Lookup(strings.Join(args, " "))
	if err != nil {
		return err
	}
	if e.g.json {
		return e.printJSON(t)
	}
	fmt.Fprintf(e.stdout, "%s\n\n%s\n\nGood: %s\n", t.Title, t.Body, t.Good)
	for _, l := range t.Links {
		fmt.Fprintf(e.stdout, "%s ↗ %s\n", l.Text, l.URL)
	}
	return nil
}
