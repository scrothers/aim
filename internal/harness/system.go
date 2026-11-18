package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// CommandTimeout bounds each external command HostSystem runs.
const CommandTimeout = 5 * time.Second

// HostSystem is the real System: it reads files, searches PATH and runs
// commands on the host.
type HostSystem struct{}

// ReadFile reads a file.
func (HostSystem) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// LookPath finds an executable on PATH.
func (HostSystem) LookPath(name string) (string, error) { return exec.LookPath(name) }

// Output runs argv with a five-second timeout and returns its stdout.
func (HostSystem) Output(ctx context.Context, argv ...string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	ctx, cancel := context.WithTimeout(ctx, CommandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
	if err != nil {
		return out, fmt.Errorf("run %s: %w", argv[0], err)
	}
	return out, nil
}
