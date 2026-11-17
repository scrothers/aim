package presence

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func kinds(evs []Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

func TestIdleStateAndEvents(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{t: t0}
	r := New(WithClock(clk.now), WithIdleAfter(10*time.Minute))
	e := r.Add("c1", preamble("s1"), t0)
	if err := e.SignOn(identity, "", t0); err != nil {
		t.Fatal(err)
	}

	clk.t = t0.Add(9 * time.Minute)
	if got := e.Status().State; got != StateOnline {
		t.Fatalf("state at 9m = %q, want online", got)
	}

	clk.t = t0.Add(11 * time.Minute)
	st := r.Online(identity)
	if len(st) != 1 || st[0].State != StateIdle || !st[0].LastActive.Equal(t0) {
		t.Fatalf("Online at 11m = %+v, want one idle session last active at t0", st)
	}
	// Reading twice records the transition once, stamped when it crossed.
	_ = r.All()
	evs := r.Events(0)
	if got := kinds(evs); len(got) != 3 || got[0] != EventWentIdle {
		t.Fatalf("events = %v, want went_idle newest of 3", got)
	}
	if !evs[0].At.Equal(t0.Add(10 * time.Minute)) {
		t.Fatalf("went_idle at %v, want %v", evs[0].At, t0.Add(10*time.Minute))
	}

	clk.t = t0.Add(12 * time.Minute)
	e.Touch(clk.t)
	if got := e.Status(); got.State != StateOnline || !got.LastActive.Equal(clk.t) {
		t.Fatalf("after Touch = %+v, want online", got)
	}
	if got := kinds(r.Events(1)); got[0] != EventActive {
		t.Fatalf("newest event = %v, want active", got)
	}
}

func TestTouchAfterUnobservedIdleRecordsBoth(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{t: t0}
	r := New(WithClock(clk.now), WithIdleAfter(time.Minute))
	e := r.Add("c1", preamble("s1"), t0)
	if err := e.SignOn(identity, "", t0); err != nil {
		t.Fatal(err)
	}
	clk.t = t0.Add(5 * time.Minute)
	e.Touch(clk.t) // nobody read presence while it was idle
	got := kinds(r.Events(2))
	if got[0] != EventActive || got[1] != EventWentIdle {
		t.Fatalf("events = %v, want active after went_idle", got)
	}
}

func TestSignedOffConnectionsEmitNoIdleEvents(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{t: t0}
	r := New(WithClock(clk.now), WithIdleAfter(time.Minute))
	e := r.Add("c1", preamble("s1"), t0)
	clk.t = t0.Add(time.Hour)
	if got := e.Status().State; got != StateIdle {
		t.Fatalf("state = %q, want idle", got)
	}
	if got := kinds(r.Events(0)); len(got) != 1 {
		t.Fatalf("events = %v, want only connected", got)
	}
}

func TestDefaultIdleAfter(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{t: t0}
	r := New(WithClock(clk.now), WithIdleAfter(0))
	e := r.Add("c1", preamble("s1"), t0)
	clk.t = t0.Add(DefaultIdleAfter - time.Second)
	if e.Status().State != StateOnline {
		t.Fatal("want online just before default threshold")
	}
	clk.t = t0.Add(DefaultIdleAfter)
	if e.Status().State != StateIdle {
		t.Fatal("want idle at default threshold")
	}
}
