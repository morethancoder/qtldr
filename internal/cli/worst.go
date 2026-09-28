package cli

import (
	"flag"
	"fmt"

	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/metrics"
)

func worstFlags(fs *flag.FlagSet) {
	fs.String("metric", metrics.CRAPMetric, "crap, coverage, mutation, cognitive or cc")
	fs.Int("n", 10, "number of functions to list")
}

func runWorst(e *env, fs *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: worst takes no arguments", errUsage)
	}
	metric := fs.Lookup("metric").Value.String()
	n := fs.Lookup("n").Value.(flag.Getter).Get().(int)
	snap, err := e.readSnapshot()
	if err != nil {
		return err
	}
	r, err := metrics.Rank(snap.Graph, metric, n, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if e.g.json {
		return e.printJSON(r)
	}
	printRanking(e, r)
	return nil
}

func printRanking(e *env, r metrics.Ranking) {
	if len(r.Items) == 0 {
		fmt.Fprintf(e.stdout, "No function has a %s value yet. %s\n", r.Metric, measureHint(r.Metric))
		return
	}
	fmt.Fprintf(e.stdout, "Worst %d functions by %s\n", len(r.Items), r.Metric)
	for i, it := range r.Items {
		stale := ""
		if it.Stale {
			stale = "  (stale)"
		}
		fmt.Fprintf(e.stdout, "%3d. %-40s %s%s\n", i+1, check.Short(it.ID), formatMetric(r.Metric, it.Value), stale)
	}
	if r.NotMeasured > 0 {
		fmt.Fprintf(e.stdout, "%d functions not measured. %s\n", r.NotMeasured, measureHint(r.Metric))
	}
}

func formatMetric(metric string, v float64) string {
	switch metric {
	case metrics.Coverage, metrics.Mutation:
		return fmt.Sprintf("%g%%", v)
	case metrics.CRAPMetric:
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%g", v)
}

func measureHint(metric string) string {
	switch metric {
	case metrics.CRAPMetric, metrics.Coverage:
		return "Run: qtldr analyze --coverage"
	case metrics.Mutation:
		return "Run: qtldr mutate"
	}
	return ""
}
