package presence

import (
	"testing"

	"github.com/scrothers/aim/internal/wire"
)

func TestAgentLookup(t *testing.T) {
	t.Parallel()
	r := New()
	r.Add("bySession", wire.Preamble{SessionID: "s1", Machine: "box"}, t0)
	old := r.Add("pidOld", wire.Preamble{Machine: "box", AgentPID: 42}, t0)
	newer := r.Add("pidNew", wire.Preamble{Machine: "box", AgentPID: 42}, t0.Add(1))
	r.Add("otherBox", wire.Preamble{Machine: "elsewhere", AgentPID: 7}, t0)

	if e, ok := r.Agent("any", "s1", 0); !ok || e.ConnID() != "bySession" {
		t.Errorf("by session = %v, %v", e, ok)
	}
	if e, ok := r.Agent("box", "", 42); !ok || e.ConnID() != newer.ConnID() {
		t.Errorf("by pid = %v, %v; want newest", e, ok)
	}
	if err := old.SignOn(identity, "", t0); err != nil {
		t.Fatal(err)
	}
	if e, ok := r.Agent("box", "", 42); !ok || e.ConnID() != old.ConnID() {
		t.Errorf("by pid with sign-on = %v, %v; want signed-on entry", e, ok)
	}
	if _, ok := r.Agent("box", "", 7); ok {
		t.Error("pid matched across machines")
	}
	if _, ok := r.Agent("box", "", 0); ok {
		t.Error("empty key matched")
	}
	if _, ok := r.Agent("box", "nope", 0); ok {
		t.Error("unknown session matched")
	}
}
