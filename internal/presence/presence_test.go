package presence

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/wire"
)

var (
	t0       = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	identity = address.MustParse("alice@github.com/acme/widgets")
	other    = address.MustParse("bob@github.com/acme/widgets")
)

func preamble(session string) wire.Preamble {
	return wire.Preamble{SessionID: session, ProjectDir: "/src/" + session, Machine: "box-" + session}
}

func TestAddRemoveLen(t *testing.T) {
	t.Parallel()
	r := New()
	if got := r.Len(); got != 0 {
		t.Fatalf("Len() = %d, want 0", got)
	}
	r.Add("c1", preamble("s1"), t0)
	r.Add("c2", preamble("s2"), t0)
	if got := r.Len(); got != 2 {
		t.Fatalf("Len() after two adds = %d, want 2", got)
	}
	// Re-adding an ID replaces the entry rather than growing the registry.
	r.Add("c1", preamble("s1b"), t0)
	if got := r.Len(); got != 2 {
		t.Fatalf("Len() after replacing c1 = %d, want 2", got)
	}
	r.Remove("c1")
	r.Remove("c1")
	r.Remove("missing")
	if got := r.Len(); got != 1 {
		t.Fatalf("Len() after removes = %d, want 1", got)
	}
}

func TestConnected(t *testing.T) {
	t.Parallel()
	r := New()
	r.Add("c1", preamble("s1"), t0)
	r.Add("c2", preamble("s1"), t0)
	if !r.Connected("s1") || r.Connected("s2") || r.Connected("") {
		t.Fatal("Connected mismatch")
	}
	r.Remove("c1")
	if !r.Connected("s1") {
		t.Error("s1 still has c2")
	}
	r.Remove("c2")
	if r.Connected("s1") {
		t.Error("s1 connected after removing all")
	}
}

func TestZeroValueRegistryAdd(t *testing.T) {
	t.Parallel()
	var r Registry
	e := r.Add("c1", preamble("s1"), t0)
	if got := r.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	if e.ConnID() != "c1" {
		t.Errorf("ConnID() = %q, want c1", e.ConnID())
	}
}

func TestEntryAccessors(t *testing.T) {
	t.Parallel()
	r := New(WithClock(func() time.Time { return t0 }))
	p := preamble("s1")
	e := r.Add("c1", p, t0)
	if e.ConnID() != "c1" {
		t.Errorf("ConnID() = %q, want c1", e.ConnID())
	}
	if e.Preamble() != p {
		t.Errorf("Preamble() = %+v, want %+v", e.Preamble(), p)
	}
	if _, ok := e.Identity(); ok {
		t.Error("Identity() ok = true before sign-on, want false")
	}
	want := Status{
		ConnID:      "c1",
		SessionID:   "s1",
		Machine:     "box-s1",
		ProjectDir:  "/src/s1",
		ConnectedAt: t0,
		State:       StateOnline,
		LastActive:  t0,
	}
	if got := e.Status(); got != want {
		t.Errorf("Status() = %+v, want %+v", got, want)
	}
}

func TestSignOnErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		addr address.Address
	}{
		{name: "namespace", addr: address.MustParse("github.com/acme")},
		{name: "repo namespace", addr: address.MustParse("github.com/acme/widgets")},
		{name: "subagent", addr: address.MustParse("sub@alice@github.com/acme/widgets")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := New()
			e := r.Add("c1", preamble("s1"), t0)
			err := e.SignOn(tt.addr, "", t0)
			if !errors.Is(err, ErrNotIdentity) {
				t.Fatalf("SignOn(%s) error = %v, want ErrNotIdentity", tt.addr, err)
			}
			if _, ok := e.Identity(); ok {
				t.Error("Identity() ok = true after failed sign-on, want false")
			}
		})
	}
}

func TestSignOnReplacesAndSignOff(t *testing.T) {
	t.Parallel()
	r := New()
	e := r.Add("c1", preamble("s1"), t0)
	if err := e.SignOn(identity, "lunch", t0); err != nil {
		t.Fatalf("SignOn() error = %v", err)
	}
	if got, ok := e.Identity(); !ok || got != identity {
		t.Fatalf("Identity() = %v, %v; want %v, true", got, ok, identity)
	}
	if err := e.SignOn(other, "", t0.Add(time.Minute)); err != nil {
		t.Fatalf("SignOn(other) error = %v", err)
	}
	st := e.Status()
	if got := r.Online(identity); len(got) != 0 {
		t.Errorf("Online(alice) after re-sign-on as bob = %d, want 0", len(got))
	}
	if got := r.Online(other); len(got) != 1 {
		t.Fatalf("Online(bob) = %d, want 1", len(got))
	}
	if st.Away != "" {
		t.Errorf("Away after re-sign-on with empty away = %q, want empty", st.Away)
	}

	e.SignOff()
	if _, ok := e.Identity(); ok {
		t.Error("Identity() ok = true after SignOff, want false")
	}
	if got := e.Status(); !got.SignedOnAt.IsZero() || got.Away != "" {
		t.Errorf("Status() after SignOff = %+v, want zero SignedOnAt and empty Away", got)
	}
	if got := r.Online(other); len(got) != 0 {
		t.Errorf("Online(bob) after SignOff = %d, want 0", len(got))
	}
	if r.Len() != 1 {
		t.Errorf("Len() after SignOff = %d, want 1; SignOff must not unregister", r.Len())
	}
}

func TestSetAway(t *testing.T) {
	t.Parallel()
	r := New()
	e := r.Add("c1", preamble("s1"), t0)
	e.SetAway("brb")
	if got := e.Status().Away; got != "brb" {
		t.Errorf("Away = %q, want brb", got)
	}
	if err := e.SignOn(identity, "", t0); err != nil {
		t.Fatalf("SignOn() error = %v", err)
	}
	e.SetAway("")
	if got := r.Online(identity); len(got) != 1 || got[0].Away != "" {
		t.Errorf("Online(alice) = %+v, want one status with empty Away", got)
	}
}

func TestOnlineFiltersAndSorts(t *testing.T) {
	t.Parallel()
	r := New()
	late := r.Add("late", preamble("s-late"), t0)
	early := r.Add("early", preamble("s-early"), t0)
	tie := r.Add("b-tie", preamble("s-tie"), t0)
	tieFirst := r.Add("a-tie", preamble("s-tie-first"), t0)
	bobConn := r.Add("bob", preamble("s-bob"), t0)
	r.Add("off", preamble("s-off"), t0)

	mustSignOn(t, late, identity, "", t0.Add(3*time.Minute))
	mustSignOn(t, early, identity, "meeting", t0.Add(1*time.Minute))
	mustSignOn(t, tie, identity, "", t0.Add(2*time.Minute))
	mustSignOn(t, tieFirst, identity, "", t0.Add(2*time.Minute))
	mustSignOn(t, bobConn, other, "", t0)

	got := r.Online(identity)
	wantIDs := []string{"early", "a-tie", "b-tie", "late"}
	if len(got) != len(wantIDs) {
		t.Fatalf("Online(alice) returned %d, want %d", len(got), len(wantIDs))
	}
	for i, id := range wantIDs {
		if got[i].ConnID != id {
			t.Errorf("Online(alice)[%d].ConnID = %q, want %q", i, got[i].ConnID, id)
		}
	}
	if got[0].Away != "meeting" {
		t.Errorf("Online(alice)[0].Away = %q, want meeting", got[0].Away)
	}
	if got[0].SessionID != "s-early" || got[0].Machine != "box-s-early" || got[0].ProjectDir != "/src/s-early" {
		t.Errorf("Online(alice)[0] preamble fields = %+v", got[0])
	}
	if !got[0].SignedOnAt.Equal(t0.Add(time.Minute)) || !got[0].ConnectedAt.Equal(t0) {
		t.Errorf("Online(alice)[0] times = %v / %v", got[0].SignedOnAt, got[0].ConnectedAt)
	}
	if got := r.Online(address.MustParse("carol@github.com/acme/widgets")); len(got) != 0 {
		t.Errorf("Online(unknown) = %d, want 0", len(got))
	}
	if got := r.Online(address.MustParse("github.com/acme/widgets")); len(got) != 0 {
		t.Errorf("Online(namespace) = %d, want 0", len(got))
	}
}

func TestOnlineEmptyRegistry(t *testing.T) {
	t.Parallel()
	if got := New().Online(identity); got == nil || len(got) != 0 {
		t.Errorf("Online() on empty registry = %#v, want empty non-nil slice", got)
	}
}

func TestConcurrentSignOnOnlineRemove(t *testing.T) {
	t.Parallel()
	r := New()
	const workers = 16
	const iterations = 200

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := "conn-" + string(rune('a'+w))
			for i := range iterations {
				e := r.Add(id, preamble(id), t0)
				addr := identity
				if i%2 == 0 {
					addr = other
				}
				if err := e.SignOn(addr, "", t0.Add(time.Duration(i))); err != nil {
					t.Errorf("SignOn() error = %v", err)
					return
				}
				e.SetAway("away")
				_ = r.Online(identity)
				_ = r.Online(other)
				_ = e.Status()
				_, _ = e.Identity()
				if i%3 == 0 {
					e.SignOff()
				}
				if i%5 == 0 {
					r.Remove(id)
				}
				_ = r.Len()
			}
		}()
	}
	wg.Wait()
	if n := r.Len(); n < 0 || n > workers {
		t.Errorf("Len() = %d, want between 0 and %d", n, workers)
	}
}

func mustSignOn(t *testing.T, e *Entry, a address.Address, away string, now time.Time) {
	t.Helper()
	if err := e.SignOn(a, away, now); err != nil {
		t.Fatalf("SignOn(%s) error = %v", a, err)
	}
}

func TestClientAndAll(t *testing.T) {
	t.Parallel()
	r := New()
	e1 := r.Add("c1", preamble("s1"), t0)
	e2 := r.Add("c2", preamble("s2"), t0.Add(time.Second))
	e2.SetClient("claude-code", "2.1")
	if err := e2.SignOn(identity, "", t0.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	all := r.All()
	if len(all) != 2 || all[0].ConnID != "c1" || all[1].ConnID != "c2" {
		t.Fatalf("All = %+v", all)
	}
	if all[0].Identity != "" || all[0].Client != "" {
		t.Errorf("unsigned status = %+v", all[0])
	}
	if got := all[1]; got.Client != "claude-code" || got.ClientVersion != "2.1" || got.Identity != identity.String() {
		t.Errorf("signed status = %+v", got)
	}
	_ = e1
}

func TestEventLog(t *testing.T) {
	t.Parallel()
	now := t0
	r := New(WithClock(func() time.Time { now = now.Add(time.Second); return now }))
	e := r.Add("c1", preamble("s1"), t0)
	e.SetClient("codex", "1")
	if err := e.SignOn(identity, "", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	e.SignOff()
	e.SignOff() // no event: already signed off
	if err := e.SignOn(identity, "", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r.Remove("c1")
	r.Remove("c1")
	ev := r.Events(0)
	kinds := make([]string, len(ev))
	for i, x := range ev {
		kinds[i] = x.Kind
	}
	want := []string{EventDisconnected, EventSignedOn, EventSignedOff, EventSignedOn, EventConnected}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if ev[0].Identity != identity.String() || ev[0].Client != "codex" || ev[0].Machine != "box-s1" {
		t.Errorf("disconnect event = %+v", ev[0])
	}
	if got := r.Events(2); len(got) != 2 || got[0].Kind != EventDisconnected {
		t.Errorf("Events(2) = %+v", got)
	}
}

func TestEventLogReplace(t *testing.T) {
	t.Parallel()
	r := New()
	e := r.Add("c1", preamble("s1"), t0)
	if err := e.SignOn(identity, "", t0); err != nil {
		t.Fatal(err)
	}
	if err := e.SignOn(other, "", t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	r.Add("c1", preamble("s2"), t0.Add(time.Minute))
	ev := r.Events(0) // newest first
	want := []struct{ kind, id string }{
		{EventConnected, ""},
		{EventDisconnected, other.String()},
		{EventSignedOn, other.String()},
		{EventSignedOff, identity.String()},
		{EventSignedOn, identity.String()},
		{EventConnected, ""},
	}
	if len(ev) != len(want) {
		t.Fatalf("events = %+v", ev)
	}
	for i, w := range want {
		if ev[i].Kind != w.kind || ev[i].Identity != w.id {
			t.Errorf("event %d = %s/%s, want %s/%s", i, ev[i].Kind, ev[i].Identity, w.kind, w.id)
		}
	}
}

func TestEventRingWraps(t *testing.T) {
	t.Parallel()
	r := New()
	for i := range maxEvents + 20 {
		r.Add(fmt.Sprintf("c%d", i), preamble("s"), t0.Add(time.Duration(i)*time.Second))
	}
	ev := r.Events(0)
	if len(ev) != maxEvents {
		t.Fatalf("len = %d, want %d", len(ev), maxEvents)
	}
	if !ev[0].At.Equal(t0.Add(time.Duration(maxEvents+19)*time.Second)) || !ev[maxEvents-1].At.Equal(t0.Add(20*time.Second)) {
		t.Errorf("newest %v oldest %v", ev[0].At, ev[maxEvents-1].At)
	}
	for i := 1; i < len(ev); i++ {
		if !ev[i].At.Before(ev[i-1].At) {
			t.Fatalf("not newest-first at %d", i)
		}
	}
}
