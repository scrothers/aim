package mcptest_test

import (
	"testing"

	"github.com/scrothers/aim/internal/mcpserver/mcptest"
)

func TestEnvClientAndHelpers(t *testing.T) {
	t.Parallel()
	e := mcptest.New(t)
	cs := e.Client(t, "a")
	if w := mcptest.OK(t, cs, "whoami", map[string]any{}); w["namespace"] != "github.com/acme/api" {
		t.Errorf("whoami = %v", w)
	}
	mcptest.Register(t, cs, "bob@github.com/acme/api", "dev")
	mcptest.Fail(t, cs, "sign_on", map[string]any{"address": "nobody@github.com/acme/api"}, "does not exist")
	if got := mcptest.Text(mcptest.Call(t, cs, "roster", map[string]any{"address": ""})); got == "" {
		t.Error("error text is empty")
	}

	e.NoNamespace = true
	if w := mcptest.OK(t, e.Client(t, "b"), "whoami", map[string]any{}); w["namespace"] != nil {
		t.Errorf("no-namespace whoami = %v", w)
	}
}
