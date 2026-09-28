package cli

import (
	"flag"
	"fmt"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/mcpserver"
)

// runMCP serves MCP over stdio. Nothing may be printed to stdout: it is the
// protocol stream.
func runMCP(e *env, _ *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: mcp takes no arguments", errUsage)
	}
	root, cfg, err := e.setup()
	if err != nil {
		return err
	}
	return mcpserver.Run(e.ctx, mcpserver.Options{Root: root, Config: cfg, Version: analysis.ToolVersion})
}
