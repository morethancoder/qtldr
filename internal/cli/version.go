package cli

import (
	"flag"
	"fmt"
	"runtime/debug"

	"github.com/morethancoder/qtldr/internal/analysis"
)

// version is the module version when installed with `go install …@vX`, else
// the source version.
func version(info *debug.BuildInfo, ok bool) string {
	if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return analysis.ToolVersion + "-dev"
}

func runVersion(e *env, _ *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: version takes no arguments", errUsage)
	}
	info, ok := debug.ReadBuildInfo()
	goVersion := ""
	if ok {
		goVersion = info.GoVersion
	}
	if e.g.json {
		return e.printJSON(map[string]string{"version": version(info, ok), "go": goVersion})
	}
	fmt.Fprintf(e.stdout, "qtldr %s (%s)\n", version(info, ok), goVersion)
	return nil
}
