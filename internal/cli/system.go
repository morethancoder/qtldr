package cli

import (
	"context"
	"os/exec"
	"strings"
)

// system is the part of the OS that doctor touches, replaced in tests.
type system interface {
	LookPath(name string) (string, error)
	Output(ctx context.Context, name string, args ...string) (string, error)
}

type osSystem struct{}

func (osSystem) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (osSystem) Output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
