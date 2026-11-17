// Command aimdev rebuilds and restarts the AIM daemon when Go sources change.
// It is development tooling: aimdev [--interval 500ms] [--root .] -- <daemon args>.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/scrothers/aim/internal/devwatch"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aimdev:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("aimdev", flag.ContinueOnError)
	interval := fs.Duration("interval", 500*time.Millisecond, "source poll interval")
	root := fs.String("root", ".", "module root to watch and build")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *interval <= 0 {
		return errors.New("--interval must be positive")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil)).With("component", "aimdev")
	return devwatch.Run(ctx, devwatch.Config{
		Root:     absRoot,
		Interval: *interval,
		Debounce: 300 * time.Millisecond,
		Bin:      filepath.Join(absRoot, ".dev", "aim-dev"),
		Args:     fs.Args(),
		Logger:   logger,
	})
}
