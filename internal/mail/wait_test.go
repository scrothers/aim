package mail_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/mail"
)

type waitOut struct {
	res mail.WaitResult
	err error
}

func (e *env) startWait(c mail.Caller, in mail.WaitInput) <-chan waitOut {
	ch := make(chan waitOut, 1)
	go func() {
		r, err := e.svc.Wait(context.Background(), c, in)
		ch <- waitOut{r, err}
	}()
	return ch
}

func expectPending(t *testing.T, ch <-chan waitOut) {
	t.Helper()
	select {
	case o := <-ch:
		t.Fatalf("wait returned early: %+v", o)
	case <-time.After(100 * time.Millisecond):
	}
}

func expectDone(t *testing.T, ch <-chan waitOut) waitOut {
	t.Helper()
	select {
	case o := <-ch:
		return o
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not return")
		return waitOut{}
	}
}

func TestWaitWakesOnSendAndDefaultsToCallerIdentity(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	ch := e.startWait(caller(bobWeb), mail.WaitInput{Timeout: 30 * time.Second})
	expectPending(t, ch)
	m := e.send(t, aliceWeb, []string{bobWeb}, "hello")
	o := expectDone(t, ch)
	if o.err != nil || o.res.TimedOut || len(o.res.Items) != 1 || o.res.Items[0].ID != m.ID {
		t.Fatalf("wait = %+v, want message %s", o, m.ID)
	}
}

func TestWaitReturnsImmediatelyWhenUnreadExists(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	m := e.send(t, aliceWeb, []string{bobWeb}, "already here")
	o := expectDone(t, e.startWait(caller(bobWeb), mail.WaitInput{Timeout: time.Minute}))
	if o.err != nil || len(o.res.Items) != 1 || o.res.Items[0].ID != m.ID {
		t.Fatalf("wait = %+v", o)
	}
}

func TestWaitSinceIDIgnoresOlderMail(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	first := e.send(t, aliceWeb, []string{bobWeb}, "first")
	ch := e.startWait(caller(bobWeb), mail.WaitInput{SinceID: first.ID, Timeout: 30 * time.Second})
	expectPending(t, ch) // "first" is unread but not newer than since_id
	second := e.send(t, aliceWeb, []string{bobWeb}, "second")
	o := expectDone(t, ch)
	if o.err != nil || len(o.res.Items) != 1 || o.res.Items[0].ID != second.ID {
		t.Fatalf("wait = %+v, want only %s", o, second.ID)
	}
}

func TestWaitScopeSubtreeAndOtherMailboxes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI)
	// Waiting on the repo namespace covers identities below it, not siblings.
	anon := mail.Caller{}
	ch := e.startWait(anon, mail.WaitInput{Address: webNS, Timeout: 30 * time.Second})
	e.send(t, aliceWeb, []string{carolAPI}, "elsewhere")
	expectPending(t, ch)
	m := e.send(t, aliceWeb, []string{bobWeb}, "in scope")
	o := expectDone(t, ch)
	if o.err != nil || len(o.res.Items) != 1 || o.res.Items[0].ID != m.ID {
		t.Fatalf("wait = %+v", o)
	}
}

func TestWaitWakesOnMarkUnread(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	m := e.send(t, aliceWeb, []string{bobWeb}, "s")
	if _, err := e.svc.Mark(ctx, caller(bobWeb), mail.MarkInput{MessageIDs: []string{m.ID}, State: mail.MarkRead}); err != nil {
		t.Fatal(err)
	}
	ch := e.startWait(caller(bobWeb), mail.WaitInput{Timeout: 30 * time.Second})
	expectPending(t, ch)
	if _, err := e.svc.Mark(ctx, caller(bobWeb), mail.MarkInput{MessageIDs: []string{m.ID}, State: mail.MarkUnread}); err != nil {
		t.Fatal(err)
	}
	if o := expectDone(t, ch); o.err != nil || len(o.res.Items) != 1 {
		t.Fatalf("wait = %+v", o)
	}
}

func TestWaitTimeoutCancelAndValidation(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, bobWeb)
	o := expectDone(t, e.startWait(caller(bobWeb), mail.WaitInput{Timeout: 100 * time.Millisecond}))
	if o.err != nil || !o.res.TimedOut || o.res.Items == nil || len(o.res.Items) != 0 {
		t.Fatalf("wait = %+v, want timed out with empty items", o)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := e.svc.Wait(ctx, caller(bobWeb), mail.WaitInput{Timeout: time.Minute})
		errc <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled wait did not return")
	}

	for name, in := range map[string]mail.WaitInput{
		"anonymous without address": {},
		"negative timeout":          {Address: webNS, Timeout: -time.Second},
		"bad since id":              {Address: webNS, SinceID: "x"},
	} {
		if _, err := e.svc.Wait(context.Background(), mail.Caller{}, in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestWaitConcurrent(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	const waiters, sends = 20, 10
	var wg sync.WaitGroup
	results := make(chan waitOut, waiters)
	for range waiters {
		wg.Go(func() {
			r, err := e.svc.Wait(context.Background(), caller(bobWeb), mail.WaitInput{Timeout: 30 * time.Second})
			results <- waitOut{r, err}
		})
	}
	time.Sleep(50 * time.Millisecond)
	var sw sync.WaitGroup
	for i := range sends {
		sw.Go(func() {
			if _, err := e.svc.Send(context.Background(), caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Subject: "s", Body: string(rune('a' + i))}); err != nil {
				t.Error(err)
			}
		})
	}
	sw.Wait()
	wg.Wait()
	close(results)
	n := 0
	for o := range results {
		n++
		if o.err != nil || o.res.TimedOut || len(o.res.Items) == 0 {
			t.Fatalf("waiter result = %+v", o)
		}
	}
	if n != waiters {
		t.Fatalf("%d waiters returned, want %d", n, waiters)
	}
}
