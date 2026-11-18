package mcptest_test

import (
	"testing"

	"github.com/scrothers/aim/internal/mcpserver/mcptest"
)

func TestEnvClientAndHelpers(t *testing.T) {
	t.Parallel()
	e := mcptest.New(t)
	cs := e.Client(t, "a")
	if w := mcptest.OK(t, cs, "agent_whoami", map[string]any{}); w["namespace"] != "github.com/acme/api" {
		t.Errorf("whoami = %v", w)
	}
	mcptest.Register(t, cs, "bob@github.com/acme/api", "dev")
	mcptest.Fail(t, cs, "agent_sign_on", map[string]any{"address": "nobody@github.com/acme/api"}, "does not exist")
	if got := mcptest.Text(mcptest.Call(t, cs, "agent_roster", map[string]any{"address": ""})); got == "" {
		t.Error("error text is empty")
	}

	e2 := mcptest.New(t)
	a, b, msgID, threadID := e2.TwoAgents(t)
	if msgID == "" || threadID == "" {
		t.Fatalf("ids = %q %q", msgID, threadID)
	}
	inbox := mcptest.ReadResource(t, b, "aim://inbox")
	mcptest.MustContain(t, "bob's inbox", inbox, "deploy plan", mcptest.Alice)
	mcptest.MustContain(t, "alice's session", mcptest.ReadResource(t, a, "aim://me"), mcptest.Alice)

	e.NoNamespace = true
	if w := mcptest.OK(t, e.Client(t, "b"), "agent_whoami", map[string]any{}); w["namespace"] != nil {
		t.Errorf("no-namespace whoami = %v", w)
	}
}
