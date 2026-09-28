package cli

import (
	"flag"
	"fmt"
	"slices"

	"github.com/morethancoder/qtldr/internal/metrics"
)

func worstFlags(fs *flag.FlagSet) {
	fs.String("metric", metrics.Cognitive, "cognitive or cc (crap, coverage and mutation need coverage and mutation runs)")
	fs.Int("n", 10, "number of functions to list")
}

func runWorst(e *env, fs *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: worst takes no arguments", errUsage)
	}
	metric := fs.Lookup("metric").Value.String()
	n := fs.Lookup("n").Value.(flag.Getter).Get().(int)
	if slices.Contains([]string{"crap", "coverage", "mutation"}, metric) {
		return fmt.Errorf("%s is not measured yet: qtldr does not run coverage or mutation in this version; use --metric cognitive or cc", metric)
	}
	snap, err := e.readSnapshot()
	if err != nil {
		return err
	}
	r, err := metrics.Rank(snap.Graph, metric, n)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if e.g.json {
		return e.printJSON(r)
	}
	fmt.Fprintf(e.stdout, "Worst %d functions by %s\n", len(r.Items), metric)
	for i, it := range r.Items {
		fmt.Fprintf(e.stdout, "%3d. %-40s %g\n", i+1, shortID(snap.Module, it.ID), it.Value)
	}
	if r.NotMeasured > 0 {
		fmt.Fprintf(e.stdout, "%d functions not measured\n", r.NotMeasured)
	}
	return nil
}
