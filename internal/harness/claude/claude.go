// Package claude integrates AIM with Claude Code: hook payload translation
// (hook.go) and settings.json installation (install.go).
package claude

// Runtime is the Claude Code integration.
type Runtime struct{}

// New returns the Claude Code integration.
func New() Runtime { return Runtime{} }

// Name returns the runtime's CLI name.
func (Runtime) Name() string { return "claude" }
