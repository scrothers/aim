// Command aim is the AIM (Agent Instant Messaging) entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var errNotImplemented = errors.New("not implemented")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := newRootCmd().ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aim:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "aim",
		Short:         "Agent Instant Messaging",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		&cobra.Command{
			Use:   "daemon",
			Short: "Run the AIM daemon",
			RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
		},
		&cobra.Command{
			Use:   "mcp",
			Short: "Run the stdio MCP shim",
			RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
		},
	)
	return root
}
