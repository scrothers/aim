package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/spf13/cobra"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/drivers"
	"github.com/scrothers/aim/internal/shim"
)

// exitError asks main to exit with a specific status without printing.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// hookDeps returns the production dependencies for a hook run.
func hookDeps() harness.Deps {
	addr := shim.DefaultAddr(os.Getenv)
	return harness.EnvDeps(
		func(ctx context.Context) (net.Conn, error) { return shim.Dial(ctx, addr) },
		func() int { return shim.AgentPID(nil) },
		shim.GitRemote,
	)
}

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hook <runtime> [event]",
		Short: "Handle an agent runtime hook (reads the payload on stdin)",
		Long: "Handle an agent runtime hook. Invoked by the runtime's hook configuration, " +
			"never by hand. Fails open: any problem exits 0 with no output.",
		Args:          cobra.RangeArgs(1, 2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHookCmd(cmd.Context(), args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), hookDeps())
		},
	}
}

func runHookCmd(ctx context.Context, args []string, in io.Reader, stdout, stderr io.Writer, deps harness.Deps) error {
	h, ok := drivers.Hook(args[0])
	if !ok {
		return nil
	}
	var event string
	if len(args) > 1 {
		event = args[1]
	}
	stdin, err := io.ReadAll(in)
	if err != nil {
		stdin = nil
	}
	out := harness.RunHook(ctx, h, event, stdin, deps)
	_, _ = stdout.Write(out.Stdout)
	_, _ = stderr.Write(out.Stderr)
	if out.Exit != 0 {
		return exitError{code: out.Exit}
	}
	return nil
}
