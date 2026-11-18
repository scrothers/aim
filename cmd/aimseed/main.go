// Command aimseed fills a development AIM database with realistic data, or
// simulates live agents against a running daemon.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scrothers/aim/internal/devseed"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aimseed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("aimseed", flag.ContinueOnError)
	db := fs.String("db", ".dev/aim.db", "SQLite database to seed (history mode)")
	addr := fs.String("addr", ".dev/aim.sock", "daemon socket path or host:port (live mode)")
	seed := fs.Uint64("seed", 1, "random seed; the same seed generates the same data")
	days := fs.Int("days", 7, "days of history")
	messages := fs.Int("messages", 400, "number of messages to generate")
	live := fs.Bool("live", false, "connect simulated agents to a running daemon instead of seeding history")
	chatter := fs.Duration("chatter", 20*time.Second, "live mode: interval between simulated actions, 0 to disable")
	force := fs.Bool("force", false, "add history even if the database already has messages")
	approvals := fs.Bool("approvals", false, "history mode: turn on remote approvals in the database settings, so a live run produces approvals")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *live {
		fmt.Println("connecting simulated agents to", *addr, "(Ctrl-C to stop)")
		err := devseed.RunLive(ctx, devseed.LiveConfig{Addr: *addr, Seed: *seed, Chatter: *chatter, Out: os.Stdout})
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}
	sum, err := devseed.SeedHistory(ctx, devseed.HistoryConfig{
		DB: *db, Seed: *seed, Days: *days, Messages: *messages, Force: *force, Approvals: *approvals,
	})
	if err != nil {
		return err
	}
	sum.Print(os.Stdout)
	return nil
}
