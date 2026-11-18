// Package harness integrates AIM with coding-agent runtimes (Claude Code,
// Codex, Gemini CLI, Grok Build, OpenCode).
//
// Each runtime is a driver subpackage implementing Hook, which translates the
// runtime's hook payloads to normalized wire.HookRequest values and the
// daemon's replies back to the runtime's stdout/exit-code contract, and
// Installer, which plans the configuration changes that register AIM's hooks
// and MCP server. The drivers subpackage is the only place that lists them.
package harness

import (
	"context"

	"github.com/scrothers/aim/internal/wire"
)

// Parsed is a runtime hook payload normalized for the daemon.
type Parsed struct {
	Request wire.HookRequest
	// SessionID is the runtime's session id from the payload or environment;
	// it becomes the hook preamble's session id.
	SessionID string
	// Cwd is the session's working directory, used for the git namespace.
	Cwd string
}

// Output is what the hook process writes and its exit code.
type Output struct {
	Stdout []byte
	Stderr []byte
	Exit   int
}

// Hook translates one runtime's hook protocol.
type Hook interface {
	// Name is the runtime's CLI name, as in `aim hook <name>`.
	Name() string
	// Client is the runtime's MCP clientInfo name prefix (e.g. "claude-code").
	Client() string
	// Parse normalizes a payload. event is the optional event argument of
	// `aim hook <name> [event]`; when empty the event comes from the payload.
	// ok is false for events AIM ignores, which exit 0 silently.
	Parse(event string, stdin []byte, getenv func(string) string) (p Parsed, ok bool, err error)
	// Render turns the daemon's reply into the runtime's output contract.
	Render(req wire.HookRequest, reply wire.HookReply) Output
}

// Scope selects which configuration an installer writes.
type Scope int

// Install scopes.
const (
	ScopeUser Scope = iota
	ScopeProject
)

// InstallOptions configures an installer plan.
type InstallOptions struct {
	// Home is the user's home directory; tests pass a temporary one.
	Home string
	// ProjectDir is the repository root for ScopeProject.
	ProjectDir string
	Scope      Scope
	// Uninstall plans removal of AIM's entries instead of adding them.
	Uninstall bool
	// MCP also registers (or unregisters) the AIM MCP server.
	MCP bool
	// Bin is the absolute path of the aim binary written into hook commands.
	Bin string
}

// Change is one planned file change. A nil After removes the file; a nil
// Before creates it.
type Change struct {
	Path   string
	Before []byte
	After  []byte
}

// Command is an external command an install runs, such as
// `claude mcp add`.
type Command struct {
	Argv []string
	// Note explains the command in dry-run output.
	Note string
}

// Plan is everything an install would do.
type Plan struct {
	Changes  []Change
	Commands []Command
	// Notes are follow-up steps for the operator, such as trusting hooks.
	Notes []string
}

// System is the host access installers need to plan, faked in tests.
type System interface {
	// ReadFile returns the file's contents, or an error satisfying
	// errors.Is(err, fs.ErrNotExist) when it is missing.
	ReadFile(path string) ([]byte, error)
	// LookPath finds an executable on PATH.
	LookPath(name string) (string, error)
	// Output runs a command and returns its standard output.
	Output(ctx context.Context, argv ...string) ([]byte, error)
}

// Installer plans one runtime's configuration changes. Plans never write;
// applying them is the caller's job.
type Installer interface {
	Name() string
	Plan(ctx context.Context, opts InstallOptions, sys System) (Plan, error)
}
