// Command aim is the AIM daemon and its stdio MCP shim.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := newRootCmd().ExecuteContext(ctx)
	stop()
	var ee exitError
	if errors.As(err, &ee) {
		os.Exit(ee.code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "aim:", err)
		os.Exit(1)
	}
}
