package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/lang/external"
	"github.com/morethancoder/qtldr/internal/lang/golang"
)

func providerFlags(fs *flag.FlagSet) {
	fs.String("ext", "", "with a command: comma-separated file extensions to send, e.g. .py,.pyi")
}

// runProvider is `qtldr provider test <name>` or `qtldr provider test
// --ext .py -- <command> [args]`: run a provider once and report contract
// problems with JSON paths. Exit 1 on problems, 2 when it cannot run.
func runProvider(e *env, fs *flag.FlagSet, args []string) error {
	if len(args) < 2 || args[0] != "test" {
		return fmt.Errorf("%w: provider test <name> | provider test --ext .py -- <command> [args]", errUsage)
	}
	root, cfg, err := e.setup()
	if err != nil {
		return err
	}
	p, err := providerFor(cfg, args[1:], fs.Lookup("ext").Value.String())
	if err != nil {
		return err
	}
	g, problems, err := external.Scan(e.ctx, root, p, cfg.Project.Exclude, golang.MatchAny)
	if err != nil {
		return err
	}
	return printProblems(e, p.Name, g, problems)
}

// providerFor finds a configured provider by name, or builds one from a
// command line.
func providerFor(cfg config.Config, args []string, ext string) (config.Provider, error) {
	if len(args) == 1 {
		for _, p := range cfg.Providers {
			if p.Name == args[0] {
				return p, nil
			}
		}
	}
	if ext == "" {
		return config.Provider{}, fmt.Errorf("%w: %q is not a provider in .qtldr.toml; to test a command, pass --ext and the command", errUsage, args[0])
	}
	return config.Provider{Name: "command", Command: strings.Join(args, " "), Args: args, Extensions: strings.Split(ext, ",")}, nil
}

func printProblems(e *env, name string, g interface{ Counts() (int, int, int) }, problems []external.Problem) error {
	if e.g.json {
		_ = e.printJSON(map[string]any{"provider": name, "ok": len(problems) == 0, "problems": problems})
	} else if len(problems) == 0 {
		n, ed, m := g.Counts()
		fmt.Fprintf(e.stdout, "Provider %s follows the contract: %d nodes, %d edges, %d metrics.\n", name, n, ed, m)
	} else {
		fmt.Fprintf(e.stdout, "Provider %s broke the contract (%d problems):\n", name, len(problems))
		for _, p := range problems {
			fmt.Fprintf(e.stdout, "  %s\n", p)
		}
	}
	if len(problems) > 0 {
		return exitCode(ExitBreach)
	}
	return nil
}
