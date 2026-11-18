package mcpserver_test

import (
	"os"
	"strings"
	"testing"
)

func TestInstructionsEmbedded(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	got := cs.InitializeResult().Instructions
	if got == "" || got != promptFile(t, "instructions.md") {
		t.Fatalf("initialize instructions = %q, want instructions.md", got)
	}
	if !strings.Contains(got, "by sending to `operator`") {
		t.Errorf("instructions lost the operator escalation guidance")
	}
}

// promptFile reads an embedded prompt's source file, trimmed the same way the
// server trims it.
func promptFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(b))
}

func TestClientInfoCaptured(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cs := e.Client(t, "a")
	if st := e.Reg.All()[0]; st.Client != "" {
		t.Fatalf("client before first call = %q", st.Client)
	}
	w := ok(t, cs, "whoami", map[string]any{})
	if w["client"] != "t" || w["client_version"] != "0" {
		t.Fatalf("whoami = %v", w)
	}
	if st := e.Reg.All()[0]; st.Client != "t" || st.ClientVersion != "0" {
		t.Errorf("status = %+v", st)
	}
	register(t, cs, "bob@github.com/acme/api", "dev")
	ok(t, cs, "sign_on", map[string]any{"address": "bob@github.com/acme/api"})
	m := ok(t, cs, "send", map[string]any{"to": []string{"operator"}, "subject": "help", "body": "need a decision"})
	if m["from_client"] != "t" || m["from_machine"] != "box" {
		t.Errorf("send = %v", m)
	}
	l := ok(t, cs, "list", map[string]any{"address": "operator"})
	if items, _ := l["items"].([]any); len(items) != 1 {
		t.Errorf("operator inbox = %v", l)
	}
}
