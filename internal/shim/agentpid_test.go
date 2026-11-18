package shim

import (
	"errors"
	"os"
	"testing"
)

func TestParseStat(t *testing.T) {
	t.Parallel()
	ppid, name, err := parseStat("123 (my (odd) proc) S 45 123 123 0 -1")
	if err != nil || ppid != 45 || name != "my (odd) proc" {
		t.Fatalf("parseStat = %d, %q, %v", ppid, name, err)
	}
	for _, bad := range []string{"", "1 (x", "1 (x) S", "1 (x) S notanumber"} {
		if _, _, err := parseStat(bad); err == nil {
			t.Errorf("parseStat(%q): want error", bad)
		}
	}
}

func TestAgentPIDSkipsShells(t *testing.T) {
	t.Parallel()
	parent := os.Getppid()
	tree := map[int]struct {
		ppid int
		name string
	}{
		parent: {ppid: 900, name: "bash"},
		900:    {ppid: 901, name: "sh"},
		901:    {ppid: 1, name: "claude"},
	}
	info := func(pid int) (int, string, error) {
		n, ok := tree[pid]
		if !ok {
			return 0, "", errors.New("missing")
		}
		return n.ppid, n.name, nil
	}
	if got := AgentPID(info); got != 901 {
		t.Fatalf("AgentPID = %d, want 901", got)
	}
}

func TestAgentPIDFallbacks(t *testing.T) {
	t.Parallel()
	failing := func(int) (int, string, error) { return 0, "", errors.New("no proc") }
	if got := AgentPID(failing); got != os.Getppid() {
		t.Errorf("AgentPID without proc = %d, want parent %d", got, os.Getppid())
	}
	toInit := func(int) (int, string, error) { return 1, "sh", nil }
	if got := AgentPID(toInit); got != 0 {
		t.Errorf("AgentPID through shells to init = %d, want 0", got)
	}
	if got := AgentPID(nil); got <= 0 {
		t.Errorf("AgentPID with real proc = %d, want > 0", got)
	}
}

func TestSessionID(t *testing.T) {
	t.Parallel()
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := SessionID(env(map[string]string{"GROK_SESSION_ID": "g", "GEMINI_SESSION_ID": "m"})); got != "m" {
		t.Errorf("SessionID = %q, want gemini first", got)
	}
	if got := SessionID(env(nil)); got != "" {
		t.Errorf("SessionID = %q, want empty", got)
	}
}
