package cli

import (
	"flag"
	"fmt"
	"runtime"

	"github.com/morethancoder/qtldr/internal/server"
	"github.com/morethancoder/qtldr/web"
)

func serveFlags(fs *flag.FlagSet) {
	fs.Bool("open", false, "open the UI in the browser")
	fs.Int("port", -1, "port on 127.0.0.1 (default: [ui].port, 0 = any free port)")
	fs.Bool("watch", false, "re-scan on file changes and push updates")
}

func runServe(e *env, fs *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: serve takes no arguments", errUsage)
	}
	root, cfg, err := e.setup()
	if err != nil {
		return err
	}
	e.info("Scanning %s…", root)
	s, err := server.New(e.ctx, server.Options{Root: root, Config: cfg, ConfigPath: e.g.config, Assets: web.Dist(), Log: e.stderr})
	if err != nil {
		return err
	}
	ln, url, err := s.Listen(servePort(fs, cfg.UI.Port))
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "qtldr is serving %s at %s (Ctrl-C to stop)\n", root, url)
	e.startExtras(s, url, boolFlag(fs, "watch"), boolFlag(fs, "open"))
	return s.Serve(e.ctx, ln)
}

// servePort is --port, else [ui].port.
func servePort(fs *flag.FlagSet, configured int) int {
	if port := fs.Lookup("port").Value.(flag.Getter).Get().(int); port >= 0 {
		return port
	}
	return configured
}

// startExtras starts the watcher and opens the browser when asked.
func (e *env) startExtras(s *server.Server, url string, watch, open bool) {
	if watch {
		go func() {
			if err := s.Watch(e.ctx); err != nil {
				fmt.Fprintln(e.stderr, "warning: --watch stopped:", err)
			}
		}()
	}
	if open {
		if err := e.sys.Start(browserCommand(url)...); err != nil {
			fmt.Fprintf(e.stderr, "warning: could not open a browser (%v); open %s yourself\n", err, url)
		}
	}
}

// browserCommand opens url with the platform's default handler.
func browserCommand(url string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"open", url}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	}
	return []string{"xdg-open", url}
}
