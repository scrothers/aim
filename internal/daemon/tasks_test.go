package daemon

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/tasks"
)

func TestSweepInterval(t *testing.T) {
	t.Parallel()
	for in, want := range map[time.Duration]time.Duration{
		tasks.DefaultIdleAfter: time.Minute,
		2 * time.Minute:        30 * time.Second,
		time.Second:            250 * time.Millisecond,
		time.Millisecond:       100 * time.Millisecond,
	} {
		if got := sweepInterval(in); got != want {
			t.Errorf("sweepInterval(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestTaskSweeperAbandons(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "aim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := mail.New(st, nil)
	const bob = "bob@github.com/acme/api"
	if _, err := svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: bob, Role: "dev"}); err != nil {
		t.Fatal(err)
	}
	var skew atomic.Int64
	clock := func() time.Time { return time.Now().Add(time.Duration(skew.Load())) }
	ts := tasks.New(st, svc, tasks.WithClock(clock), tasks.WithDecay(tasks.Decay{IdleAfter: 400 * time.Millisecond}))
	c := mail.Caller{SessionID: "s1", Identity: address.MustParse(bob), SignedOn: true}
	tk, err := ts.Create(ctx, c, tasks.CreateInput{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Update(ctx, c, tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim}); err != nil {
		t.Fatal(err)
	}
	changed := ts.Changed()
	skew.Store(int64(tasks.DefaultAbandonAfter + time.Minute))

	d := &daemon{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), tasks: ts}
	sctx, cancel := context.WithCancel(ctx)
	done := d.startTaskSweeper(sctx)
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper did not run")
	}
	cancel()
	<-done
	got, err := ts.Get(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != tasks.StatusAbandoned {
		t.Errorf("status = %s, want abandoned", got.Status)
	}
}
