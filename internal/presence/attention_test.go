package presence

import (
	"testing"
	"time"

	"github.com/scrothers/aim/internal/wire"
)

func TestSetAttention(t *testing.T) {
	t.Parallel()
	r := New(WithClock(func() time.Time { return t0 }))
	e := r.Add("c", wire.Preamble{Machine: "box"}, t0)

	// Not signed on: the state is kept but no event is logged.
	if since := e.SetAttention(wire.AttentionWaiting, "idle", t0); !since.Equal(t0) {
		t.Errorf("since = %v", since)
	}
	if n := len(r.Events(10)); n != 1 { // connected
		t.Errorf("events = %d, want 1", n)
	}
	if err := e.SignOn(identity, "", t0); err != nil {
		t.Fatal(err)
	}

	t1, t2 := t0.Add(time.Second), t0.Add(2*time.Second)
	e.SetAttention(wire.AttentionBlocked, "first", t1)
	if since := e.SetAttention(wire.AttentionBlocked, "second", t2); !since.Equal(t1) {
		t.Errorf("repeat since = %v, want %v", since, t1)
	}
	st := e.Status()
	if st.Attention != wire.AttentionBlocked || st.AttentionDetail != "second" || !st.AttentionSince.Equal(t1) {
		t.Errorf("status = %q %q %v", st.Attention, st.AttentionDetail, st.AttentionSince)
	}
	evs := r.Events(10)
	if last := evs[0]; last.Kind != EventAttention || last.Attention != wire.AttentionBlocked || last.Identity != identity.String() {
		t.Errorf("last event = %+v", last)
	}
	attention := 0
	for _, ev := range evs {
		if ev.Kind == EventAttention {
			attention++
		}
	}
	if attention != 1 {
		t.Errorf("attention events = %d, want 1 (repeats are not logged)", attention)
	}

	if since := e.SetAttention("", "", t2); !since.IsZero() {
		t.Errorf("cleared since = %v", since)
	}
	if st := e.Status(); st.Attention != "" || st.AttentionDetail != "" || !st.AttentionSince.IsZero() {
		t.Errorf("cleared status = %+v", st)
	}

	e.SetAttention(wire.AttentionFailed, "boom", t2)
	e.Touch(t2)
	if st := e.Status(); st.Attention != "" {
		t.Errorf("after Touch attention = %q", st.Attention)
	}
}
