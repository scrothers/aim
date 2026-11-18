package mcpserver_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/mcptest"
)

const (
	alice = mcptest.Alice
	bob   = mcptest.Bob
)

// Short names for the shared test helpers.
var (
	newEnv   = mcptest.New
	ok       = mcptest.OK
	text     = mcptest.Text
	register = mcptest.Register
)

func twoAgents(t *testing.T, e *mcptest.Env) (a, b *mcp.ClientSession, msgID, threadID string) {
	t.Helper()
	return e.TwoAgents(t)
}
