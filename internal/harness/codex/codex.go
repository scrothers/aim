// Package codex integrates AIM with the OpenAI Codex CLI: hook payload
// translation (hook.go) and hooks.json installation (install.go).
package codex

// Runtime is the Codex integration.
type Runtime struct{}

// New returns the Codex integration.
func New() Runtime { return Runtime{} }

// Name returns the runtime's CLI name.
func (Runtime) Name() string { return "codex" }
